# Changelog

All notable changes to Loom will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.10] - 2026-02-24

### Added
- `public-relations-manager` added to `allowed_roles` in `config.yaml` — enables automatic creation of PR manager agents for all projects (loom, aviation, tokenhub) on startup; agents now monitor GitHub issues, PRs, and community interactions via the motivation system
- Filed beads for all open GitHub issues and PRs across loom (PRs #23, #24, #25), tokenhub (issues #3, #4, #7 and PRs #5, #6, #8, #9), and Aviation (issues #109, #110 and 23 Dependabot PRs)

### Changed
- `internal/dispatch/dispatcher.go`: `rolesMatch(a, b string) bool` helper added to normalize role name comparisons, replacing repeated inline `normalizeRoleName()` calls in `dispatch_phases.go`

### Fixed
- `make clean` now removes `site/` MkDocs build artifact directory

## [0.1.9] - 2026-02-23

### Added
- `loomctl bead errors <id>` — decodes and displays a bead's `error_history` in a readable per-dispatch table with timestamps and truncated error messages; also shows `dispatch_count`, `last_run_error`, `ralph_blocked_reason`, and `loop_detected_reason`
- `loomctl bead unblock <id>` — shorthand for redispatching a blocked bead with an optional `--reason` flag
- `loomctl provider list/show/register/delete` — provider management commands restored to source (existed in installed binary but had been lost from `main.go`, meaning any rebuild would silently drop them)
- `tokenhubctl model-test <id> [api-key]` — fires a real inference request through a specific model and reports status code, latency, token usage, and partial response; accepts key via argument, `TOKENHUB_API_KEY` env, or falls back to the admin token
- `tokenhubctl provider-status <id>` — shows full health detail for a single provider: state, request/error counts, average latency, last success time, last error message and timestamp, and cooldown expiry

### Fixed
- `tokenhubctl health` was displaying `-` for the ERRORS and LAST SUCCESS columns due to wrong field names (`consecutive_errors` / `last_success` instead of the API's actual `consec_errors` / `last_success_at`); columns now show correct values and a new LAST ERROR column has been added
- NATS consumer leak on restart: `Close()` now calls `conn.Drain()` with a 5-second deadline instead of iterating `Unsubscribe()`, cleanly unbinding durable consumers before the TCP connection drops and preventing "already bound" errors on the next startup
- NATS "already bound" recovery: `subscribe()` now detects the `already bound` error, deletes the stale durable consumer from the previous run, and retries once, preventing cascading context-canceled failures when the PDA orchestrator starts up
- `dispatch_phases.go`: `isProviderError` call-sites corrected after upstream signature changed from `error` to `string` — previously `isProviderError(execErr)` and `isProviderError(fmt.Errorf(...))` caused compile errors
- TaskExecutor `executeBead` return value: added missing `return false` on the success path after signature changed to `(needsBackoff bool)`
- Worker backoff: TaskExecutor workers now pause 3 seconds after a provider error instead of immediately retrying, preventing the 50+ RPS spin that saturated tokenhub's 60 RPS/IP rate limit

### Changed
- Ralph `LoomHeartbeatActivity` Phase 3 (DispatchOnce loop) is now a no-op: it was spawning goroutines tied to the Temporal activity context, causing all inflight tasks to receive `context.Canceled` every ~10 seconds when the activity completed (~1 300 errors/hour); TaskExecutor handles all execution directly
- Ralph auto-recovery now includes `rate limit` in the set of transient block reasons eligible for 30-minute cooldown reset
- Ralph auto-recovery for auth-blocked beads: beads blocked for authentication errors are now eligible for automatic reset after 2 hours (key rotation and temporary outages can cause auth failures that clear on their own)
- CEO REPL: agent dropdown added for targeted bead dispatch to a specific agent

## [0.1.8] - 2026-02-23

### Changed
- GitHub Pages now built with MkDocs (Material theme) instead of a bespoke Python script that referenced the long-deleted `docs/USER_GUIDE.md`; Pages deployment re-enabled via GitHub Actions
- Build gate: exit 127 (toolchain not found) now **blocks** commits instead of silently allowing them through; agent receives step-by-step instructions to detect and install the required toolchain then verify the build before retrying
- Build gate: build failure message now explicitly says "DO NOT call done or git_commit until the build passes" and provides the full compiler/tool output — prevents agents from giving up without fixing the error

### Fixed
- `docs/archive/` broken links no longer cause `mkdocs build` warnings (excluded via `exclude_docs`)
- `pymdownx` removed from Pages workflow pip install (it ships inside `mkdocs-material`, not as a standalone package)

## [0.1.7] - 2026-02-23

### Added
- Executor idle/sleep mode: worker goroutines exit after 3 minutes of no work (36 × 5s idle rounds) instead of spinning forever, eliminating idle token spend
- Per-project watcher goroutine (long-lived): polls for ready beads every 30 seconds and respawns workers immediately when work appears
- Periodic git fetch in watcher: every 5 minutes, fetches the project's beads-sync branch and reloads if new commits are found — detects beads pushed externally without needing an API call
- `WakeProject(projectID)` on the executor and `Loom`: signals via buffered channel for zero-latency wake on bead creation or beads reset

### Changed
- `POST /api/v1/beads` (bead creation) now calls `WakeProject` after creating a bead, ensuring sleeping workers restart within milliseconds
- `POST /api/v1/projects/{id}/beads/reset` also calls `WakeProject` after reload

### Fixed
- Agent-introduced build errors: `sync.Map` used as `map[string]int` for `actionHashes` and `actionTypeCount`, `handleTokenLimits` called with extra argument, `if` block body left orphaned in `handlers_analytics.go`

## [0.1.6] - 2026-02-22

### Added
- Project beads reset API: `POST /api/v1/projects/{id}/beads/reset` clears in-memory bead state and reloads from the beads-sync branch — essential for recovering from dolt migrations, force-pushed beads worktrees, or other out-of-band changes
- `loomctl project reset-beads <project-id>` subcommand to invoke the reset from the CLI
- `loomctl container` subcommands: `list`, `logs`, `restart`, `status` for managing project containers
- `loomctl bead list` now supports `--status`, `--type`, `--assigned-to`, and `--priority` filters

### Changed
- Replaced Temporal/NATS dispatch loop with direct `TaskExecutor` — worker goroutines now claim and run beads without going through Temporal workflows or NATS routing, eliminating the primary source of dispatch failures
- Bead status persistence hardened: closing a bead now always clears `assigned_to`; stale `open+assigned` beads are auto-reset on next claim attempt

### Fixed
- Beads completing work but remaining `open` due to `assigned_to` not being cleared on status update
- Dispatch starvation: skip open beads whose assigned agent is busy rather than blocking the entire queue
- Agent-introduced build errors repaired: duplicate const blocks, `sync.Map` used as regular map, `Priority *int` type mismatch, orphaned error-handling statements, missing braces, undefined sentinel errors

## [0.1.5] - 2026-02-22

### Added
- Self-audit runner: periodically runs build/test/lint and files beads for failures (`SELF_AUDIT_INTERVAL_MINUTES`)
- Auto-merge runner: squash-merges approved agent PRs (`AUTO_MERGE_INTERVAL_MINUTES`)
- Sentinel errors `ErrBeadNotFound`, `ErrBeadAlreadyClaimed`, `ErrFileLocked` for structured error handling
- `SetUseNATSDispatch()` method on Dispatcher for per-instance NATS routing control

### Fixed
- Dispatcher task goroutine leaked on context cancellation (used `context.Background()` instead of request ctx)
- Commit lock held indefinitely when context cancelled mid-acquisition in `acquireCommitLock`
- Redispatch/escalate API endpoints silently ignored malformed JSON request bodies (now return 400)
- `os.WriteFile` debug calls in dispatch hot path silently swallowed errors and wrote to `/tmp`
- JSON unmarshal errors in loop detector `getErrorHistory`/`hasRecentProgress` silently returned empty values
- Error classification in bead API handlers used fragile `strings.Contains` matching (now uses `errors.Is`)
- `UseNATSDispatch` package-level global prevented test isolation (moved to struct field)
- NATS JetStream "consumer already bound" error resolved by scoping `ConsumerPrefix` to `ServiceID`
- Dispatch pipeline: stale beads reassigned, terminal beads skipped, remediation cascade capped (15-min cooldown, max 3 per bead, CEO escalation on exhaustion)
- Project initialization duplicated `loom` project entry on restart

## [0.1.4] - 2026-02-21

### Added
- config: move tokenhub secrets to ~/.loom/config.env, add compose profile
- add TokenHub to observability UI and documentation

### Changed
- remove provider UI from web dashboard
- simplify temporal heartbeat and remove loomctl provider commands
- strip multi-provider logic from loom core and database
- simplify dispatcher to pick first active provider
- simplify provider registry and model for TokenHub-only
- delete scoring, complexity, GPU selection, and Ollama provider code
- delete routing package and remove API endpoints

### Fixed
- prevent runaway bead dispatches with tighter inflight guard and hard upper bound (bd-106)
- update provider in-place in registry to prevent stale worker pointers (bd-105)
- use chat completion probe for provider health check and preserve API key on PUT
- align loop detector tests with read-only progress semantics

### Other
- config: rename TOKENHUB_URL/API_KEY to generic LOOM_PROVIDER_* names
- tokenhub: enable vault, drop hardcoded sparky endpoints

## [0.1.3] - 2026-02-21

### Added
- integrate TokenHub as LLM routing layer
- project-scoped filtering across all resources
- loomctl P0 beads, priority fix, agent self-improvements
- comprehensive Makefile + full Helm chart for k8s deployment
- LLM system state endpoint GET /api/v1/system/state
- intelligent per-project memory system
- GitHub integration via gh CLI
- multi-role in-container orchestration
- phase 2 - NATS RPC package + service discovery wiring
- phase 1 - build system and code TODO resolution
- autonomous self-healing system + fix pre-existing test failures
- Add MIT license file to repository
- Create VERSION file with content 0.1.0
- implement container agent registration and sync exec
- enforce pre-commit build gate with per-repo discovery
- add use_container field to PUT /api/v1/projects/{id}
- add Kubernetes manifests and Linkerd service mesh (Phase 5)
- implement ConnectorsService gRPC server (Phase 4)
- add PgBouncer connection pooler in front of loom-postgresql
- wire NATS message bus through dispatcher and containers
- add observability stack and connector service proto
- implement NATS-based async agent communication
- implement Phase 2 - PostgreSQL support
- implement Phase 1 - NATS message bus foundation
- implement location-transparent connector architecture
- implement token-based auth and per-project container orchestration
- Phase 1 - Project Agent Service implementation
- execute bootstrap.local at container startup
- add analytics velocity command and improve install target
- implement headless UI bug reporter with auto-bead filing
- add change velocity metrics and dashboard widget (loom-016 Phase 2)
- add comprehensive linting for code quality (catch dumb bugs)
- increase iteration budget and add checkpoint commits (loom-016 Phase 1)
- make start and restart depend on build
- improve stop command and add prune target
- add export/import UI to web interface
- add database export/import API and CLI for backup/migration
- v2.0 rewrite with JSON-first output and full API coverage
- add bead poke command for redispatching stuck beads
- implement loomctl CLI tool for Loom server interaction
- implement self-healing system for stuck agents
- disable DoltCoordinator to let bd manage Dolt
- build bd from source with CGO for Dolt support
- increase max loop iterations from 25 to 50
- integrate OpenTelemetry stack with Jaeger, Prometheus, and Grafana
- auto-advance workflows to first node on creation
- implement workflow start API endpoint
- enable autonomous agent commits with proper attribution
- implement agent role inference for workflow routing (Gap #3)
- implement commit serialization to prevent git conflicts (Gap #2)
- implement multi-dispatch redispatch flag
- initialize workflow system implementation with waterfall workflow
- add OpenClaw messaging bridge for P0 decision escalations
- add test coverage analysis with 75% threshold requirement
- migrate personas to Agent Skills specification format
- increase max_iterations from 15 to 25 for complex tasks
- prevent agent assignment for infrastructure beads

### Changed
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- [WIP] Auto-checkpoint after file changes
- drop SQLite, require PostgreSQL only
- upgrade to modern v13 schema with tsdb index
- implement git-centric architecture with worktrees
- switch from /usr/local/bin to ~/.local/bin

### Fixed
- allow batch mode to proceed with test warnings
- make docs target graceful when mkdocs is not installed
- resolve golangci-lint findings across codebase
- skip Helm templates in yaml-lint, allow root-level MEMORY.md
- remove dead priority type comparison and unused package vars
- Use pointer types for int fields in CreateMotivationRequest
- Improve diagnostic bead description and fix loomctl syntax error
- stuck inner_loop/progress_stagnant beads re-dispatch infinitely
- allow priority 0 (P0) in bead creation
- rename projectid loom-self → loom across all beads
- prevent agent explosion on restart
- remove maxAgents cap from RestoreAgentWorker + add staleness timeout
- Create AUTONOMY_TEST.md with required content
- eliminate agent explosion, stale agents, and bd errors
- remove artificial pool limit; add container agent registration retry
- graceful degradation + arch-aware Go + UseContainer persistence
- return 404 for project agent assign/unassign when project not found
- return 404 instead of 500 for GET workflow not found
- return 404 for bead redispatch/escalate when bead not found
- return 404 instead of 500 for PATCH bead not found
- return 404 instead of 500 for DELETE project not found
- add version and uptime to GET /api/v1/health response
- return 503 instead of 500 when worker pool is at capacity
- return 404 instead of 500 for DELETE provider not found
- improve analytics and manager nil diagnostics
- fix all PostgreSQL test failures and achieve full test suite pass
- port log manager SQL to PostgreSQL and re-enable interceptor
- add retry logic for container startup connection issues
- disable structured metadata for compatibility with older schema
- resolve lock contention causing API timeouts
- correct command flags for NATS container
- implement per-project git storage configuration
- set project working directory for agent command execution
- run bootstrap.local on HOST using loomctl
- make workflow enforcement opt-in to unblock dispatch
- detect and clear dead agent assignments after restart
- stop remediation cascade and fix parse error messages
- make worker pool SpawnWorker idempotent
- auto-configure upstream tracking for beads branches
- implement write-through scoring to eliminate dual data model
- detect repeated infrastructure errors as stuck
- persist beads database across container restarts
- initialize noop tracer to prevent test crashes
- change default working directory from /app/src to /app
- increase max import size from 50MB to 200MB
- build both loom and loomctl, preserve databases in clean
- implement -o table output format
- decouple agents from providers, round-robin across pool
- detect scope loops early and fix bead loading fallback
- raise stagnation threshold and fix action type names
- persist git_auth_method and reset workflows on redispatch
- set loom-self project to SSH auth for git push
- use simple mode for 30B-class models, raise context threshold
- align all SKILL.md files with simple JSON format and fix role names
- resolve 7 critical disconnects preventing autonomous self-modification
- add read_bead_conversation to action schema prompt
- add YAML frontmatter to remediation-specialist persona
- add remediation-specialist to allowed roles
- wire up BeadReader interface to enable conversation access
- add missing actions for reading bead conversations
- include stdout/stderr in bash command results
- resolve worker deadlock and context cancellation blocking auto-dispatch
- allow dispatch for escalated workflows + skip system beads
- identify workflow escalation blocking dispatch
- add error logging and debug tracing for dispatch loop
- disable federation sync to prevent startup hang
- disable entrypoint Dolt startup, use DoltCoordinator
- correct loom-self beads path in entrypoint
- include .git in image for loom-self bead loading
- copy .git directory for loom-self project
- copy .beads directory to runtime image
- correct Loom UI port in observability menu
- bump app.js version to force cache refresh
- correct observability menu links and icons
- disable bd CLI build due to upstream import cycle errors
- add input validation to StartWorkflow function
- update tests to match new role inference behavior
- repair broken tests in api, loom, and consensus packages
- resolve deadlock in AgentMessageBus.Close()
- use directory path as persona Name for backward compatibility
- increase HTTP client timeout from 5min to 15min for action loops
- configure loom-self for local development mode
- stop infinite redispatch loop for beads that hit max_iterations
- ensure deps installs required go toolchain
- improve deps setup across os and dolt downloads
- beads never unassigned — auto-assign to CTO/EM, add CTO persona

### Other
- Microservices phases 4-5, documentation overhaul, and Loom personality
- Remove mistaken case of loom server

## [0.1.2] - 2026-02-12

### Added
- conversations redesign — Cytoscape action-flow graph
- D3.js visualization layer — donuts, bars, gauges, sparklines, treemaps
- Dolt multi-reader/multi-writer coordinator for per-project beads
- Action progress tracker, conversation viewer, CEO REPL fix
- Pre-push test gate — build and test must pass before git push

### Fixed
- users page blank when auth disabled, logs UI blank, duplicate formatAgentDisplayName
- error toast cascade, bead modal layout, default dispatch to Engineering Manager
- UI improvements — agent names, bead modal, kanban project filter, spawn naming
- last lint — ineffectual output assignment in branch delete
- lint round 3 — staticcheck, ineffassign, gosimple
- remaining lint errors — unused funcs, errcheck, gosimple
- CI failures — test, lint errcheck, gosec SARIF upload
- auto-file circuit breaker, bd auto-init, bootstrap health check
- bead creation fallback, conversation UI field mapping
- streaming timeouts, SSH deploy key handling, conversations UI
- CEO REPL agent routing and context-aware queries
- CEO REPL bead creation uses agent project_id, skips auto-file

## [0.1.0] - 2026-02-10

### Added
- Give agents project context and action bias in dispatch
- Remove P0 dispatch filter, add connector + MCP + OpenClaw beads
- Round-robin dispatch across equal-score providers
- Phases 2-5 — edit matching, spatial awareness, feedback, lessons
- Simple JSON mode — 10 actions with response_format constraint
- Text-based action system for local model effectiveness
- Add NVIDIA cloud provider + env var expansion in config
- Enable structured JSON output for local LLM providers
- Dolt bootstrap in entrypoint + SSH key isolation
- Dolt SQL server in container entrypoint + federation enabled
- important note from Loom's co-creator
- Decouple containerized loom from host source mount
- Multi-turn action loop engine — close the LLM feedback loop
- Git expertise — merge, revert, checkout, log, fetch, branch ops + AGENTS.md procedures
- Pair-programming mode — streaming chat with agents scoped to beads
- Ralph Loop — heartbeat-driven work draining, CEO role restriction
- Wire observability endpoints with event ring buffer and analytics logging
- Auto-assign providers to agents from shared pool
- Unified bead viewer/editor modal with agent dispatch
- Add GitStrategy model + UI, simplify docker SSH mounts
- Migrate beads to Dolt backend + P2P federation support
- SSH key bootstrap + DB persistence, comprehensive documentation overhaul
- Complete Phase 3 CRUD enhancements - Provider & Persona edit
- Complete Phase 1 & 2 of UI overhaul - branding, core features
- Complete project bootstrap feature (Phase 2 - Full Implementation)
- Implement project bootstrap backend (Phase 1)
- enable autonomous operation by delegating to management agents
- enable autonomous multi-agent operation
- implement graceful escalation with comprehensive context
- implement smart loop detection
- increase dispatch hop limit from 5 to 20
- Implement Agent Delegation with task decomposition
- Add Consensus Decision Making system
- Implement Shared Bead Context for agent collaboration
- Add ActionSendAgentMessage for inter-agent communication
- Implement Agent Message Bus infrastructure
- update code reviewer persona with PR review workflow
- implement PR review actions for code review workflow
- complete PR event listener with tests and docs
- implement PR event listener for code review workflow
- Add refactoring, file management, debugging, and docs actions (ac-r60.2-5)
- Add code navigation actions (LSP integration) (ac-r60.1)
- Add workflow integration and PR creation (ac-qv6x, ac-5yu.5)
- Add git commit/push actions for agent workflows
- Implement GitService layer for agent git operations
- Add feedback loop orchestration system
- Add ActionBuildProject verification system
- Implement linter integration for automated code quality checks
- Add ActionRunTests to agent action schema
- Implement TestRunner service with multi-framework support
- Register conversation API routes in server
- Add conversation context API handlers with session management
- add conversation session management to Dispatcher
- add conversation history support to Worker
- implement ConversationContext model for multi-turn conversations
- Add comprehensive agentic enhancement roadmap (48 beads)
- implement provider substitution recommendations
- implement prompt optimization for cost reduction
- implement email notifications for budget alerts
- add authentication and permission filtering to activity feed
- add CI/CD pipeline and webhook notifications
- add usage pattern analysis and optimization engine
- add activity feed and notifications system
- add cache opportunities analyzer
- add commenting and discussion threads for beads
- expose project readiness in state endpoint
- add structured agent action logging
- show project git key in settings
- enable project git readiness and ssh keys
- add batching recommendations
- enhance workflow diagrams UI
- implement workflow system Phase 5 - Advanced Features
- implement workflow system Phase 4 - REST API and Visualization UI
- complete workflow system Phase 3 - all safety features implemented
- implement workflow system Phase 3 - Safety & Escalation (ac-1453, ac-1455)
- implement workflow system Phase 2 - Dispatcher Integration (ac-1480, ac-1486)
- implement workflow system Phase 1 (ac-1450, ac-1451, ac-1452)
- enable multi-turn agent investigations via in_progress redispatch
- auto-create apply-fix beads when CEO approves code fixes
- integrate hot-reload into main application
- implement hot-reload system for development
- add bug investigation workflow for agents
- implement auto-bug dispatch system for self-healing
- implement perpetual tasks for proactive agent workflows
- add interactive workflow diagram visualization

### Changed
- Move providers out of config.yaml to API-based bootstrap
- README with additional context on Loom
- README to clarify Loom's self-maintenance
- note from Loom's co-creator in README
- Update beads config prefix from ac to loom
- Final beads DB + JSONL agenticorp cleanup
- Fix beads data after daemon restart
- Fix ac-4yo9 bead ID reference in description text
- Replace agenticorp in compacted beads data (skip hooks)
- Final cleanup of agenticorp refs in beads data
- Replace agenticorp references in main beads content
- Replace agenticorp references in app/src bead content
- Rename personas/agenticorp to personas/loom
- Rename .agenticorp directory to .loom
- Rename app/src bead prefix bd- to loom-
- Rename bead prefix ac- to loom-, clean up artifacts
- Complete agenticorp → loom rename across entire codebase
- Rename AgentiCorp to Loom throughout codebase

### Fixed
- Add panic recovery to dispatch loop goroutine
- Add nil guard and startup log to dispatch loop
- Async task execution in DispatchOnce, set WorkDir on projects
- Set WorkDir on managed projects so refresh and dispatch use correct paths
- Filter beads by project prefix to prevent cross-project dispatch
- Preserve bead project_id across refresh cycles
- Match beads to agents by project affinity in dispatch
- Remove artificial max_concurrent agent limit
- Don't terminate action loop when close_bead fails
- Always run dispatch loop, don't gate on Temporal availability
- Periodic bead cache refresh so externally-created beads get dispatched
- Enable no-auto-import so Dolt is single source of truth for beads
- Pass API key through syncRegistry so Protocol has auth for completions
- Persist and read context_window in provider DB queries
- Capture context window in Protocol heartbeat path too
- Use discovered context window instead of hardcoded model limits
- Initialize key manager before Temporal activities registration
- Dispatch loop fills all idle agents per tick, not just one
- Wire API key through heartbeat probe for cloud providers
- Close 813 noise beads in JSONL, fix Dolt zombie in entrypoint
- Make start/stop/restart use Docker, not native binary
- API key flows through provider registration to Protocol + heartbeat
- Auto-rediscover model on 404 instead of marking provider unhealthy
- Readiness failures auto-file as P3, not P0
- Path audit + provider capability scoring
- SimpleJSON parser accepts both new and legacy action formats
- Increase provider HTTP timeout from 60s to 5min
- Stronger ACTION instruction + example in text prompt
- Separate validation errors from parse failures + dispatch cooldown
- Fall through to any-agent dispatch when workflow role unavailable
- Deadlock in ResetStuckAgents + only load active beads
- Only load active beads into memory, not 4600+ closed ones
- Heartbeat uses provided URL path instead of scanning ports
- Provider activation accepts both 'active' and 'healthy' status + doc updates
- Always run bd init for schema creation, stash before pull
- Set upstream tracking branch after init+fetch clone
- Handle non-empty project dir during clone + init beads after clone
- Use direct DB query for agent lookup in pair handler
- Bootstrap modal CSS dark theme -> light theme palette
- Prevent all reloads/renders while modal is open
- Modal stability, all bead fields, suppress auth errors
- Stop UI bead-storm from 4xx API errors on polling loop
- Detect local project by beads path presence, not just git_repo
- Standardize provider status on "healthy", remove dead activation loop
- Remove last ac-4yo9 reference from beads JSONL
- Phase 1 - Update branding from AgentiCorp to Loom
- prioritize self-improvement workflow for tagged beads
- auto-activate providers on startup regardless of status
- move documentation files to docs/ directory per repo rules
- resolve 2 High severity security vulnerabilities
- resolve 3 Critical security vulnerabilities from audit
- build beads bd CLI from source in Docker
- skip persona tests requiring missing persona files
- resolve test timeout by skipping slow integration tests
- resolve remaining lint errors and add security audit
- resolve golangci-lint errors (unused functions and errcheck)
- implement GitOperator interface methods in gitops.Manager
- resolve gosimple, ineffassign, and staticcheck linting errors
- remove remaining unused functions and fix test errcheck violations
- resolve all linting errors (errcheck and unused functions)
- resolve final errcheck violations in test files
- resolve all remaining errcheck linting violations
- add error checking for remaining linting violations
- add error checking to test files for linting compliance
- build golangci-lint from source for Go 1.25 compatibility
- resolve CI/CD build failures
- resolve critical work flow blockers preventing dispatch
- escalate beads after dispatch hop limit
- auto-enable redispatch for open beads
- use bd cli for bead persistence
- remove auth headers when auth disabled
- auto-file api failures as p0 ui bugs
- warn on provider endpoints and normalize bead prefixes
- stabilize docs CI checks
- stabilize metrics, dispatch routing, and gitops commits
- make clean now excludes agenticorp-self directory
- remove QA Engineer pre-assignment to enable auto-routing
- extract history array from motivations API response
- resolve duplicate API_BASE declaration breaking UI

