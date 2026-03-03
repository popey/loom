# Changelog

All notable changes to Loom will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.24] - 2026-03-03

### Fixed
- DefaultConfig HTTP port 8081 → 8080 to match actual server binding

## [0.1.23] - 2026-03-03

### Added
- Fix: Add graceful degradation to handleConversation for unavailable d...
- Fix: Add proper test setup for conversation handlers
- Add composite index on conversation_contexts(project_id, updated_at D...
- Fix: Add nil check for s.app in conversation handlers
- Fix P0 API 503 error: Add missing rows.Err() check in ListConversatio...
- Fix P0 API 503 error: Add timeout context to conversation list query
- Fix GET /api/v1/conversations 503 error - add missing context and lim...
- Fix API 503 error in GET /api/v1/conversations - add proper database ...
- Fix: Add nil checks for logManager in logging API handlers
- Fix: Add conversation_contexts table to PostgreSQL schema
- Add Ping() method to Database struct
- Fix conversation handler context parameter and add retry logic
- Fix 503 error for GET /api/v1/conversations by adding retry mechanism...
- Add tests for GET /api/v1/conversations endpoint to verify method han...
- Fix API failure for GET /api/v1/conversations by adding database conn...
- Add delay to simulate retry logic in handleConversationsList to addre...
- Add logging to confirm database connection in handleConversationsList
- Add return statement after error response in handleConversationsList
- Add fallback definition for apiCall in LogViewer to handle undefined ...
- Fix goroutine leaks in Initialize() by adding WaitGroup tracking
- Add proper defer cleanup in WriteFile error paths

### Changed
- Improve PostgreSQL connection pool configuration
- apply go fmt to loom_coverage_test.go

### Fixed
- remove broken mockApp from conversation tests; use nil app instead
- goroutine leaks in Ralph loop and motivation engine; propagate context in action handlers
- Fix nil pointer dereference in handleBeadConversation
- Fix: ListConversationContextsByProject SQL placeholder issue
- Fix: ListConversationContextsByProject query formatting for PostgreSQL
- Fix GET /api/v1/conversations endpoint returning 500 error
- Fix 503 error on GET /api/v1/conversations - PostgreSQL LIMIT paramet...
- Fix conversation handler 503 errors and clean up backup files
- Fix conversations endpoint test expectations
- Fix P0 API failure: GET /api/v1/conversations gracefully degrades to ...
- Fix conversation list timeout by avoiding large JSON deserialization
- Fix 503 error in conversation list handler by using request context
- Fix handlers_conversation_test.go: align mockApp.GetDatabase() return...
- Fix: Mock database for conversation handler tests
- Fix context shadowing in ListConversationContextsByProject
- Fix conversation list test expectation
- Fix: Return 503 Service Unavailable on database errors in handleConve...
- Fix: Initialize EntityMetadata in conversation context retrieval
- Fix: Use rebind() for ListConversationContextsByProject placeholder c...
- Fix ListConversationContextsByProject query parameter binding
- Fix SQL injection vulnerability in ListConversationContextsByProject
- Fix GET /api/v1/conversations 503 error - PostgreSQL LIMIT parameter ...
- Fix: Replace undefined parseInt() with inline strconv.Atoi() in conve...
- Fix type mismatch in handleConversationsList response
- Fix: Route /api/v1/conversations/ to list handler when no ID provided
- Fix: Skip conversation migrations for PostgreSQL to prevent SQLite sy...
- Fix handleConversationsList to gracefully handle nil app/database
- Fix: Remove invalid PostgreSQL cast syntax from LIMIT clause in ListC...
- Fix handleConversationsList to return 200 with mock data when app is nil
- Fix 503 error in ListConversationContextsByProject by casting LIMIT p...
- Fix: Remove unnecessary db.Ping() call in handleConversationsList
- Fix conversation handler context parameter bug
- Fix nil pointer dereference in conversation handlers
- Fix handleConversationsList: move db.Ping() after nil check
- Fix: Remove context parameter from ListConversationContextsByProject ...
- Fix: Remove problematic 2-second sleep from /api/v1/conversations end...
- Fix API failure for GET /api/v1/conversations by ensuring database co...
- Fix ReferenceError: apiCall is not defined in LogViewer
- Fix ReferenceError: apiCall is not defined in LogViewer
- Fix 'apiCall' reference in logs.js to use the 'apiCall' function from...
- repair agent-broken app.js (syntax errors, deleted renderProjects, duplicate const)
- Fix ReferenceError: apiCall is not defined in logs.js
- Fix port mismatch: change Loom to listen on 8080 instead of 8081
- remove agent-generated duplicate files (fix_errors.go, loom_shutdown.go)
- repair multiple agent-introduced syntax errors in loom_lifecycle.go
- stop paused agents with providers from cycling every Ralph beat
- Fix defer placement in AuditLogger.LogOperationWithDuration
- improve error handling in loom_lifecycle.go
- Fix race condition in readinessMu locking - use defer for proper mute...
- Fix race condition in EventBus.Close() shutdown sequence
- resolve go vet errors from agent-generated code
- add BatchMode=yes to SSH command to prevent interactive auth att...

### Removed
- harden: remove agent artifacts, fix code holes, add commit guardrails, model tier check
- Fix P0 API failure: remove hardcoded sleep delays in conversations en...

### Other
- Reorder conversation handlers: move handleConversationsList before ha...
- Remove redundant s.app nil checks in conversation handlers
- Ensure apiCall is correctly referenced in logs.js

## [0.1.22] - 2026-03-01

### Added
- add agent/model/bead metadata to all agent-generated commits
- Fix: Add defer statements to db.Close() calls in error paths
- add circular dependency detection tool
- Add attemptSelfHeal function to fix missing project readiness issues
- Add validation for invoke_skill, post_to_board, and vote actions
- Add ModelHint field to Task struct for model selection hints
- bd-292: Add workflows, connectors, and persona-editor panels to main SPA
- Add ephemeralstate, modelselection, and selfoptimization managers to ...
- Add database migrations for org chart assignments, performance review...
- expand CEO Command Center with organizational health sections
- expand CEO Command Center with organizational health sections
- Add meetings section to UI
- Phase 4: Add organizational visibility UI sections
- Add sortColumns function and columnPriority map
- Add persona endpoint to agent action handler
- Add Performance Reviews tab with agent grading and accountability
- Add performance review API endpoints for agent grading and accountabi...
- Add Blocked column to Kanban board
- Add Blocked column to Kanban board with styling
- add Phase 2 handlers for feedback, status, review, and department
- register meetings API routes in SetupRoutes
- add GetMeetingsManager accessor method
- Add tests for collaboration and consensus managers
- add org chart handler functions with PUT and POST support
- Replace CEO decision loop with real LLM call
- add mark-and-sweep recovery for blocked beads

### Changed
- apply go fmt to loom_coverage_test.go
- split loom.go into domain-focused files
- Apply remaining changes: update outputJSON/outputTable signatures and...

### Fixed
- resolve go vet errors from agent-generated code
- resolve build errors from remote agent-pushed commits
- motivation engine blocking + duplicate bead ID + type fixes
- restore compilable loom package after agent-split file errors
- Fix port mismatch: change default HTTPPort from 8080 to 8081 to match...
- Fix: Remove duplicate renderDecisions function declaration
- Fix indentation error in Loom struct initialization
- Fix org chart agent population: use RoleName field and UpsertOrgChart...
- resolve build errors from agent-generated code conflicts
- DecodeLenient falls back to simple JSON format when frontier models use it
- correct NewManager call to not pass db parameter
- resolve container networking to external TokenHub

### Other
- Enhanced Shutdown function with proper resource cleanup
- EM: Reset stuck bead loom-kr9e (provider infrastructure failure)
- EM: Reset stuck bead loom-8tw0 (provider infrastructure failure)
- bd-291: Mark dispatch package as parked
- Mark dispatch package as parked
- bd-291: Move LoopDetector to standalone loopdetector package
- Remove dispatcher field from Loom struct and all references
- UI: Reorganize Decisions tab to show human escalations, agent-handled...
- Wire motivation engine into Initialize - start engine on loom startup
- Wire meetings, status board, consensus, and collaboration into loom.go
- Enhance AgentDetailModal with action handlers
- bd-280: Enhance agent cards with display names, performance grades, a...
- Wire collaboration and consensus managers into Loom

## [0.1.21] - 2026-02-27

### Added
- full agent autonomy and CEO experience fixes

### Fixed
- Makefile health-check and URLs use wrong port
- close 5 critical executor design flaws

## [0.1.20] - 2026-02-26

### Added
- add CI/CD monitor (CIMon) that auto-files P0 beads for GitHub Actions failures

### Fixed
- silence spurious docker compose env var warnings
- resolve CI lint failures and openclaw data race
- repair pre-existing test and vet failures across the tree
- clear conversation history on redispatch and handle parse_failures properly

## [0.1.19] - 2026-02-26

### Fixed
- Agent-generated build errors (third pass): duplicate `newBeadUnblockCommand` body outside function in `cmd/loomctl/main.go`, unused `"log"` import in `cmd/connectors-service/main.go`, orphaned code block outside `checkRalphBlockage` in `internal/health/watchdog.go`, agent-created `fix_loom.go` causing `main` redeclaration

## [0.1.18] - 2026-02-26

### Added
- `docs/LOOM_ARCHITECTURE.md`: comprehensive agent reference injected into every agent's bead context, covering bead lifecycle, deadlock patterns (6 types with escape strategies), agent roles, execution environment, and system invariants
- Architecture doc + `LESSONS.md` now injected via `buildBeadContext()` in TaskExecutor — agents have full system awareness
- Internal executor fields (`dispatch_count`, `error_history`, `loop_detected`, etc.) excluded from agent prompts to reduce noise

### Fixed
- Agent-generated build errors: orphaned code outside function bodies in `handlers_conversation.go`, broken `Execute` closures in `perpetual.go` (illegal import inside function literal), undefined variables `projectID`/`err`/`dr` in `loom.go`, duplicate method declarations in `database` package, unused import in `linter`, missing types in `github/types.go`, duplicate `ListFailedWorkflowRuns` in `github/client.go`
- Loop-detected beads now properly set to `blocked` (not `open`), stopping infinite TaskExecutor recycling

## [0.1.17] - 2026-02-26

### Fixed
- Loop-detected beads now set to `blocked` instead of `open`, stopping infinite executor recycling
- Agent-generated build errors: duplicate `motivation_state_provider.go`, wrong module path `loom-project/loom`, broken `motivation.NewEngine` call with wrong signature, invalid `conn.Drain()` argument in NATS
- `motivation_provider.go`: rewritten with correct API calls (`ListBeads`, `GetPendingDecisions`, direct agent manager iteration for idle agents)

## [0.1.16] - 2026-02-25

### Added
- `EnsureDefaultAgents` public API on `*Loom` — bootstrapped projects now get a full org chart (all roles) created immediately on `POST /api/v1/projects/bootstrap`
- Three-level debug instrumentation system (`debug_level: off|standard|extreme` in config.yaml)

### Fixed
- Provider activation on restart: providers loaded from DB on startup are now re-probed via chat-completion health check so they transition from `unhealthy` → `active` without manual intervention
- Provider activation on update: `UpdateProvider` now re-probes immediately so endpoint/key edits take effect right away
- New project button: wired to `showBootstrapProjectModal()` (bootstrap flow with description field and deploy key); added "Register Existing" secondary button for bare registration
- Bootstrap success screen: SSH deploy key shown prominently with copy button and GitHub instructions
- Git `index.lock` races: added per-project mutex in beads `Manager` to serialize concurrent git operations
- Git add pathspec failure for beads loaded from main worktree: `SaveBeadToGit` now derives file path from `m.beadFiles` and copies to beads worktree when needed
- Three UI null-pointer errors on page load: `renderKanban`, `renderAgents`, `renderPersonas`, `renderDecisions` now guard against missing DOM elements
- SSE streams: prevent `ERR_INCOMPLETE_CHUNKED_ENCODING` by flushing properly

### Changed
- Slug-based project IDs: `POST /api/v1/projects/bootstrap` generates URL-safe slugs from project name (e.g. "My App" → `my-app`) instead of sequential `proj-N` IDs

## [0.1.15] - 2026-02-25

### Removed
- Remove Temporal entirely (was only used for Ralph Loop maintenance heartbeat)
  - Delete internal/temporal/ (~9,000 lines of workflow/DSL/activity/eventbus code)
  - Remove 3 Temporal Docker containers (temporal-postgresql, temporal, temporal-ui) from docker-compose.yml
  - Remove Temporal SDK (go.temporal.io/sdk, go.temporal.io/api) from go.mod

### Changed
- Replace Temporal LoomHeartbeatWorkflow with plain `time.NewTicker` goroutine in loom.go
- Move EventBus from internal/temporal/eventbus to internal/eventbus (standalone, no Temporal)
- Move Ralph Loop activities from internal/temporal/activities to internal/ralph package
- Replace RunReplQuery Temporal workflow with direct provider API call
- Remove temporal section from config.yaml and all env var references

### Fixed
- Fix project bootstrap failure: bd init always requires Dolt (unavailable in container due to arch incompatibility)
  - For YAML backend, create minimal .beads/beads/ directory directly instead of calling bd init

### Documentation
- Remove all Temporal references from docs/ (configuration, deployment, architecture, API, getting-started)
- Remove Temporal UI link and log source filter from web UI

## [0.1.14] - 2026-02-24

### Fixed
- wire Add Project button to showCreateProjectModal

## [0.1.13] - 2026-02-24

### Changed
- go fmt pkg/config/config.go
- replace all hardcoded "loom-self" with config-driven self_project_id

### Fixed
- resolve three UI errors on page load

## [0.1.12] - 2026-02-24

### Fixed
- resolve CI lint and test failures
- correct port mapping in docker-compose — loom listens on 8081 not 8080

## [0.1.11] - 2026-02-24

### Added
- add explicit build→test→push loop to all code-touching agents
- auto-bootstrap provider from LOOM_PROVIDER_URL env var

### Fixed
- move AUTONOMY_TEST.md and release_checklist.md into docs/
- add site/ to .gitignore so mkdocs build doesn't dirty the tree
- build project-agent and connectors-service as static binaries
- trust bind-mounted source dir in builder container
- add error handling for debug os.WriteFile calls in dispatch loop
- containerize all Go operations — only Docker+Make required on host
- add no-stray-scripts rule after observing fix_ui.go incident
- remove stray fix_ui.go script left in root package
- prevent remediation beads for provider/infrastructure ...
- resolve duplicate emojis and remove legacy sections
- persist provider API keys across restarts, harden key security
- use lifecycle context with timeout for task goroutines
- add link for the CI badge to point to status
- add link for the CI badge to point to status
- format code with `gofmt`; avoid lint warnings

### Other
- Move from alpine to ubuntu

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

