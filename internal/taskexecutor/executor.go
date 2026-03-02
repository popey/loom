// Package taskexecutor provides a direct bead execution engine that uses
// named agents from the WorkerManager. It spawns goroutines per project that
// claim beads, match them to agents by role, and run them through the agent's
// worker. Anonymous fallback workers are used when no named agent is available.
package taskexecutor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"encoding/json"
	"errors"

	"sort"

	"github.com/google/uuid"
	"github.com/jordanhubbard/loom/internal/actions"
	"github.com/jordanhubbard/loom/internal/beads"
	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/loopdetector"
	"github.com/jordanhubbard/loom/internal/persona"
	"github.com/jordanhubbard/loom/internal/project"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/internal/worker"
	"github.com/jordanhubbard/loom/pkg/models"
)

const (
	defaultNumWorkers = 5
	// maxIdleRounds: after this many consecutive nil-claim rounds (each 5s),
	// a worker goroutine exits. 36 × 5s = 3 minutes of idleness.
	maxIdleRounds = 36
	// watcherInterval: how often the watcher checks for new work when idle.
	watcherInterval = 30 * time.Second
	// gitFetchInterval: how often the watcher does a git fetch to detect
	// beads pushed from external sources.
	gitFetchInterval = 5 * time.Minute
	// recoverySweepInterval: how often the watcher runs the blocked-bead
	// recovery sweep. Must be long enough that transient provider outages
	// have time to resolve before we re-queue work.
	recoverySweepInterval = 5 * time.Minute
	// zombieBeadThreshold: if an in_progress bead with an ephemeral exec-*
	// assignment has not been updated in this long, its executor goroutine
	// is considered dead and the bead is reclaimed.
	zombieBeadThreshold = 30 * time.Minute
	// providerErrorBackoff: how long a worker pauses after a provider error
	// (502, 429, context canceled) before claiming the next bead. Prevents
	// hot-spin loops that exhaust the tokenhub rate limit (60 RPS / IP).
	providerErrorBackoff  = 3 * time.Second
	maxConcurrentRequests = 3
	// irrecoverableDispatchThreshold: a bead that has been dispatched this
	// many times across distinct workers without any of them being infra
	// errors is considered irrecoverable and gets escalated to the CEO.
	irrecoverableDispatchThreshold = 15
)

// CEOEscalator is called when a bead is irrecoverably stuck and must be
// escalated to the CEO for a human decision.
type CEOEscalator interface {
	EscalateBeadToCEO(beadID, reason, returnedTo string) error
}

// AgentManager is the interface for the WorkerManager that the executor uses
// to look up named agents, update their status, and match beads to agents.
type AgentManager interface {
	ListAgentsByProject(projectID string) []*models.Agent
	ListAgents() []*models.Agent
	GetAgent(id string) (*models.Agent, error)
	UpdateAgentStatus(id, status string) error
	AssignBead(agentID, beadID string) error
	GetIdleAgentsByProject(projectID string) []*models.Agent
}

// OrgChartProvider gives the executor access to the org chart for role-based
// routing and manager escalation.
type OrgChartProvider interface {
	GetOrgChart(projectID string) *models.OrgChart
	GetDefaultOrgChart() *models.OrgChart
}

// projectState tracks per-project executor state.
type projectState struct {
	activeWorkers  int
	watcherRunning bool
	// wakeCh is sent on to immediately unblock a sleeping watcher.
	wakeCh chan struct{}
	// execMu serializes bead execution per project. Multiple workers can
	// poll and claim beads concurrently, but only one can execute filesystem
	// operations (git, file edits, builds) at a time to prevent corruption.
	execMu sync.Mutex
}

// Executor is the direct bead execution engine.
type Executor struct {
	providerRegistry *provider.Registry
	beadManager      *beads.Manager
	actionRouter     *actions.Router
	projectManager   *project.Manager
	personaManager   *persona.Manager
	agentManager     AgentManager
	orgChart         OrgChartProvider
	ceoEscalator     CEOEscalator
	reviewManager    *ReviewManager
	db               *database.Database
	lessonsProvider  worker.LessonsProvider
	numWorkers       int
	projectStates    map[string]*projectState
	semaphore        chan struct{}
	mu               sync.Mutex
}

// New creates an Executor.
func New(
	providerRegistry *provider.Registry,
	beadManager *beads.Manager,
	actionRouter *actions.Router,
	projectManager *project.Manager,
	db *database.Database,
) *Executor {
	return &Executor{
		providerRegistry: providerRegistry,
		beadManager:      beadManager,
		actionRouter:     actionRouter,
		projectManager:   projectManager,
		db:               db,
		numWorkers:       defaultNumWorkers,
		projectStates:    make(map[string]*projectState),
		semaphore:        make(chan struct{}, maxConcurrentRequests),
	}
}

// SetPersonaManager wires in the persona loader for rich agent instructions.
func (e *Executor) SetPersonaManager(pm *persona.Manager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.personaManager = pm
}

// SetCEOEscalator wires in the callback for irrecoverable bead escalation.
func (e *Executor) SetCEOEscalator(esc CEOEscalator) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ceoEscalator = esc
}

// SetAgentManager wires in the WorkerManager so the executor uses named agents.
func (e *Executor) SetAgentManager(am AgentManager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agentManager = am
}

// SetOrgChart wires in the org chart provider for role-based routing.
func (e *Executor) SetOrgChart(oc OrgChartProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.orgChart = oc
}

// StartReviewSystem initializes and launches the weekly performance review loop.
// Agents are graded A-F based on bead completion rates. Agents with consecutive
// D/F scores are asked to self-optimize their persona files, and those that fail
// to improve after multiple cycles are fired.
func (e *Executor) StartReviewSystem(ctx context.Context) {
	e.mu.Lock()
	am := e.agentManager
	pm := e.personaManager
	esc := e.ceoEscalator
	e.mu.Unlock()

	if am == nil {
		log.Printf("[Reviews] Review system not started: no agent manager configured")
		return
	}

	rm := NewReviewManager(am, e.beadManager, pm)
	if esc != nil {
		rm.SetCEOEscalator(esc)
	}

	e.mu.Lock()
	e.reviewManager = rm
	e.mu.Unlock()

	go rm.StartReviewLoop(ctx)
}

// GetReviewManager returns the review manager for API access.
func (e *Executor) GetReviewManager() *ReviewManager {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reviewManager
}

// SetLessonsProvider wires in the lessons provider for build failure learning.
func (e *Executor) SetLessonsProvider(lp worker.LessonsProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lessonsProvider = lp
}

// SetNumWorkers sets the number of concurrent worker goroutines per project.
func (e *Executor) SetNumWorkers(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.numWorkers = n
}

// Start ensures the watcher is running and spawns workers for projectID.
// Safe to call multiple times; spawns workers only when none are active.
func (e *Executor) Start(ctx context.Context, projectID string) {
	e.mu.Lock()
	state := e.getOrCreateState(projectID)
	n := e.numWorkers

	// Start the long-lived watcher if not already running
	if !state.watcherRunning {
		state.watcherRunning = true
		e.mu.Unlock()
		go e.watcherLoop(ctx, projectID)
	} else {
		e.mu.Unlock()
	}

	// Spawn workers up to numWorkers
	e.mu.Lock()
	toSpawn := n - state.activeWorkers
	state.activeWorkers += toSpawn
	e.mu.Unlock()

	if toSpawn > 0 {
		log.Printf("[TaskExecutor] Spawning %d worker(s) for project %s", toSpawn, projectID)
		for i := 0; i < toSpawn; i++ {
			go e.workerLoop(ctx, projectID)
		}
	}
}

// WakeProject signals that new work may be available, spawning workers if idle.
func (e *Executor) WakeProject(projectID string) {
	e.mu.Lock()
	state := e.getOrCreateState(projectID)
	ch := state.wakeCh
	e.mu.Unlock()

	// Non-blocking send: watcher may already be awake
	select {
	case ch <- struct{}{}:
	default:
	}
}

// getOrCreateState returns the projectState for projectID, creating it if needed.
// Caller must hold e.mu.
func (e *Executor) getOrCreateState(projectID string) *projectState {
	if s, ok := e.projectStates[projectID]; ok {
		return s
	}
	s := &projectState{
		wakeCh: make(chan struct{}, 1),
	}
	e.projectStates[projectID] = s
	return s
}

// workerLoop claims and executes beads. Exits after maxIdleRounds of no work.
func (e *Executor) workerLoop(ctx context.Context, projectID string) {
	workerID := fmt.Sprintf("exec-%s-%s", projectID, uuid.New().String()[:8])
	log.Printf("[TaskExecutor] Worker %s started for project %s", workerID, projectID)

	idleRounds := 0
	defer func() {
		e.mu.Lock()
		if s, ok := e.projectStates[projectID]; ok {
			s.activeWorkers--
			if s.activeWorkers < 0 {
				s.activeWorkers = 0
			}
		}
		e.mu.Unlock()
		log.Printf("[TaskExecutor] Worker %s exiting (idle=%d)", workerID, idleRounds)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Acquire semaphore slot for concurrency limiting
		select {
		case <-ctx.Done():
			return
		case e.semaphore <- struct{}{}:
		}

		bead := e.claimNextBead(ctx, projectID, workerID)
		if bead == nil {
			<-e.semaphore // Release semaphore slot
			idleRounds++
			if idleRounds >= maxIdleRounds {
				log.Printf("[TaskExecutor] Worker %s idle for %ds, going to sleep",
					workerID, idleRounds*5)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		idleRounds = 0
		log.Printf("[TaskExecutor] Worker %s claimed bead %s (%s)", workerID, bead.ID, bead.Title)

		// Acquire per-project execution lock. Workers can claim beads
		// concurrently but must serialize filesystem operations (git,
		// file edits, builds) to prevent workspace corruption.
		e.mu.Lock()
		state := e.getOrCreateState(projectID)
		e.mu.Unlock()
		state.execMu.Lock()

		needsBackoff := e.executeBead(ctx, bead, workerID)

		state.execMu.Unlock()
		<-e.semaphore // Release semaphore slot

		if needsBackoff {
			select {
			case <-ctx.Done():
				return
			case <-time.After(providerErrorBackoff):
			}
		}
	}
}

// watcherLoop runs forever for a project. It wakes workers when new beads arrive,
// either from the API (via WakeProject) or from a periodic git fetch.
func (e *Executor) watcherLoop(ctx context.Context, projectID string) {
	log.Printf("[TaskExecutor] Watcher started for project %s", projectID)
	defer log.Printf("[TaskExecutor] Watcher stopped for project %s", projectID)

	ticker := time.NewTicker(watcherInterval)
	defer ticker.Stop()
	gitTicker := time.NewTicker(gitFetchInterval)
	defer gitTicker.Stop()
	recoveryTicker := time.NewTicker(recoverySweepInterval)
	defer recoveryTicker.Stop()

	e.mu.Lock()
	state := e.getOrCreateState(projectID)
	wakeCh := state.wakeCh
	e.mu.Unlock()

	// Run an initial recovery sweep at startup so beads blocked by a
	// previous provider outage are unblocked immediately.
	e.recoverBlockedBeads(projectID)

	for {
		select {
		case <-ctx.Done():
			return

		case <-wakeCh:
			// Immediate wake signal from API (new bead created or beads reloaded)
			e.maybeSpawnWorkers(ctx, projectID)

		case <-ticker.C:
			// Periodic check: any ready beads?
			e.maybeSpawnWorkers(ctx, projectID)

		case <-gitTicker.C:
			// Periodic git fetch to detect beads pushed externally
			e.fetchAndReloadBeads(ctx, projectID)
			e.maybeSpawnWorkers(ctx, projectID)

		case <-recoveryTicker.C:
			// Mark-and-sweep: re-open blocked beads whose failure reason
			// was transient (provider errors, rate limits). Escalate truly
			// irrecoverable beads to the CEO.
			e.recoverBlockedBeads(projectID)
			e.maybeSpawnWorkers(ctx, projectID)
		}
	}
}

// maybeSpawnWorkers spawns workers for projectID if there is work and none are active.
func (e *Executor) maybeSpawnWorkers(ctx context.Context, projectID string) {
	readyBeads, err := e.beadManager.GetReadyBeads(projectID)
	if err != nil || len(readyBeads) == 0 {
		return
	}

	e.mu.Lock()
	state := e.getOrCreateState(projectID)
	n := e.numWorkers
	toSpawn := n - state.activeWorkers
	if toSpawn <= 0 {
		e.mu.Unlock()
		return
	}
	state.activeWorkers += toSpawn
	e.mu.Unlock()

	log.Printf("[TaskExecutor] Waking %d worker(s) for project %s (%d ready beads)",
		toSpawn, projectID, len(readyBeads))
	for i := 0; i < toSpawn; i++ {
		go e.workerLoop(ctx, projectID)
	}
}

// fetchAndReloadBeads does a git fetch on the project's beads worktree and reloads
// if the remote has new commits.
func (e *Executor) fetchAndReloadBeads(ctx context.Context, projectID string) {
	beadsPath := e.beadManager.GetProjectBeadsPath(projectID)
	if beadsPath == "" {
		return
	}
	// The beads path is the .beads directory inside the worktree.
	// Git operations run on the parent (the worktree root).
	worktreeRoot := filepath.Dir(beadsPath)

	// Get current HEAD before fetch
	headBefore, err := os.ReadFile(filepath.Join(worktreeRoot, ".git", "HEAD"))
	if err != nil {
		// Not a git worktree or no HEAD — skip
		return
	}

	// Fetch from origin (non-blocking: if it fails we skip gracefully)
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := fmt.Sprintf("cd %q && git fetch origin 2>/dev/null", worktreeRoot)
	if err := runShell(fetchCtx, cmd); err != nil {
		return
	}

	// Check FETCH_HEAD vs current HEAD
	fetchHead, err := os.ReadFile(filepath.Join(worktreeRoot, ".git", "FETCH_HEAD"))
	if err != nil {
		return
	}
	if strings.TrimSpace(string(headBefore)) == strings.TrimSpace(strings.Fields(string(fetchHead))[0]) {
		return // No new commits
	}

	// New commits: reset to FETCH_HEAD and reload
	resetCtx, cancel2 := context.WithTimeout(ctx, 30*time.Second)
	defer cancel2()
	if err := runShell(resetCtx, fmt.Sprintf("cd %q && git reset --hard FETCH_HEAD 2>/dev/null", worktreeRoot)); err != nil {
		log.Printf("[TaskExecutor] git reset failed for project %s: %v", projectID, err)
		return
	}

	log.Printf("[TaskExecutor] New beads detected for project %s, reloading", projectID)
	e.beadManager.ClearProjectBeads(projectID)
	if err := e.beadManager.LoadBeadsFromGit(ctx, projectID, beadsPath); err != nil {
		log.Printf("[TaskExecutor] reload failed for project %s: %v", projectID, err)
	}
}

// runShell executes a shell command, returning an error on non-zero exit.
func runShell(ctx context.Context, cmd string) error {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	return c.Run()
}

// recoverBlockedBeads is the mark-and-sweep recovery mechanism. It scans all
// blocked beads for the project and classifies each as recoverable or
// irrecoverable.
//
// Recoverable (re-opened): beads blocked by transient infrastructure failures
// (provider 502s, rate limits, budget exhaustion). These are the common case
// after a provider outage resolves.
//
// Irrecoverable (escalated to CEO): beads that have been dispatched to many
// distinct workers, all of which failed with non-infrastructure errors. This
// means every agent that tried has given up — only a human can unblock it.
func (e *Executor) recoverBlockedBeads(projectID string) {
	allBeads, err := e.beadManager.ListBeads(map[string]interface{}{
		"project_id": projectID,
		"status":     models.BeadStatusBlocked,
	})
	if err != nil {
		return
	}

	recovered := 0
	escalated := 0
	for _, b := range allBeads {
		if b == nil || b.Status != models.BeadStatusBlocked {
			continue
		}
		if b.Context == nil {
			b.Context = map[string]string{}
		}

		// Skip beads already escalated to CEO
		if b.Context["escalated_to_ceo_at"] != "" {
			continue
		}

		if isTransientFailure(b) {
			// Transient infra error: reset to open, clear loop state, let
			// the next worker pick it up with a clean slate.
			_ = e.beadManager.UpdateBead(b.ID, map[string]interface{}{
				"status":      models.BeadStatusOpen,
				"assigned_to": "",
				"context": map[string]string{
					"loop_detected":        "false",
					"loop_detected_reason": "",
					"loop_detected_at":     "",
					"recovery_reason":      "transient infrastructure failure resolved — re-queued by recovery sweep",
					"recovered_at":         time.Now().UTC().Format(time.RFC3339),
				},
			})
			recovered++
		} else if isIrrecoverable(b) {
			// Every agent tried and failed with non-infra errors. Escalate.
			e.mu.Lock()
			esc := e.ceoEscalator
			e.mu.Unlock()

			reason := fmt.Sprintf("All agents failed on bead %s (%s) after %s dispatches. Last error: %s",
				b.ID, b.Title, b.Context["dispatch_count"], truncate(b.Context["last_run_error"], 200))

			if esc != nil {
				if err := esc.EscalateBeadToCEO(b.ID, reason, ""); err != nil {
					log.Printf("[TaskExecutor] Failed to escalate bead %s to CEO: %v", b.ID, err)
				} else {
					escalated++
				}
			} else {
				log.Printf("[TaskExecutor] Bead %s is irrecoverable but no CEO escalator configured: %s", b.ID, reason)
			}
		}
		// Beads that are neither transient nor irrecoverable are left blocked.
		// They'll be re-evaluated on the next sweep as more context accumulates.
	}

	if recovered > 0 || escalated > 0 {
		log.Printf("[TaskExecutor] Recovery sweep for %s: %d recovered, %d escalated to CEO",
			projectID, recovered, escalated)
	}
}

// isTransientFailure returns true if the bead was blocked by infrastructure
// errors that are expected to resolve on their own (provider outages, rate
// limits, budget exhaustion).
func isTransientFailure(b *models.Bead) bool {
	reason := b.Context["loop_detected_reason"]
	if reason == "" {
		reason = b.Context["loop_detection_reason"]
	}
	if reason == "" {
		return false
	}
	reasonLower := strings.ToLower(reason)
	for _, pattern := range []string{
		"provider error",
		"provider unavailable",
		"rate limit",
		"budget exceeded",
		"provider quota",
		"exhausted provider",
		"all providers fa",
		"failed to send request",
		"dial tcp",
		"connection refused",
		"no such host",
		"i/o timeout",
		"context deadline exceeded",
		"eof",
		"502",
		"503",
		"504",
	} {
		if strings.Contains(reasonLower, pattern) {
			return true
		}
	}
	return false
}

// isIrrecoverable returns true if the bead has been attempted by many distinct
// workers and all failed with non-infrastructure errors — meaning no agent can
// make progress and a human must intervene.
func isIrrecoverable(b *models.Bead) bool {
	dc := 0
	fmt.Sscanf(b.Context["dispatch_count"], "%d", &dc)
	if dc < irrecoverableDispatchThreshold {
		return false
	}

	// Check error history for diversity of workers and non-infra errors.
	type errRecord struct {
		Timestamp string `json:"timestamp"`
		Error     string `json:"error"`
		Dispatch  int    `json:"dispatch"`
	}
	var history []errRecord
	if raw := b.Context["error_history"]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &history)
	}

	// Count how many recent errors are NOT infra errors.
	// If the majority are logic/agent failures, the bead is irrecoverable.
	nonInfraErrors := 0
	for _, h := range history {
		errLower := strings.ToLower(h.Error)
		isInfra := false
		for _, pattern := range []string{"502", "429", "all providers", "budget", "rate limit", "provider"} {
			if strings.Contains(errLower, pattern) {
				isInfra = true
				break
			}
		}
		if !isInfra {
			nonInfraErrors++
		}
	}

	// At least half the error history must be non-infra for irrecoverable
	return len(history) > 0 && nonInfraErrors*2 >= len(history)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// claimNextBead returns the next available bead for the project, or nil.
// Beads are sorted by priority (P0 first) so urgent work is claimed first.
func (e *Executor) claimNextBead(ctx context.Context, projectID, workerID string) *models.Bead {
	_ = ctx // reserved for future use
	readyBeads, err := e.beadManager.GetReadyBeads(projectID)
	if err != nil {
		log.Printf("[TaskExecutor] GetReadyBeads(%s) error: %v", projectID, err)
		return nil
	}

	// Sort by priority ascending (P0=0 is most urgent).
	sort.Slice(readyBeads, func(i, j int) bool {
		if readyBeads[i] == nil || readyBeads[j] == nil {
			return readyBeads[i] != nil
		}
		return readyBeads[i].Priority < readyBeads[j].Priority
	})

	for _, b := range readyBeads {
		if b == nil {
			continue
		}
		// Decision beads that explicitly require human authority (real-world
		// spending, token purchases, etc.) are not auto-claimable.
		if b.Type == "decision" && b.Context["requires_human"] == "true" {
			continue
		}
		// Rescue zombie in-progress beads. Ephemeral executor IDs (exec-<project>-<uuid>)
		// are created per goroutine and die without cleanup when loom restarts or the
		// goroutine is killed. If the bead has not been updated in zombieBeadThreshold,
		// the executor is gone and we reset it back to open so it can be reclaimed.
		if b.Status == models.BeadStatusInProgress && b.AssignedTo != "" {
			if strings.HasPrefix(b.AssignedTo, "exec-") && time.Since(b.UpdatedAt) > zombieBeadThreshold {
				log.Printf("[TaskExecutor] Reclaiming zombie bead %s (stale executor %s, age %v)",
					b.ID, b.AssignedTo, time.Since(b.UpdatedAt).Round(time.Second))
				_ = e.beadManager.UpdateBead(b.ID, map[string]interface{}{
					"status":      models.BeadStatusOpen,
					"assigned_to": "",
				})
				b.Status = models.BeadStatusOpen
				b.AssignedTo = ""
			} else {
				continue
			}
		}
		// Fix inconsistent state: open bead with stale assignment — reset it
		if b.Status == models.BeadStatusOpen && b.AssignedTo != "" {
			_ = e.beadManager.UpdateBead(b.ID, map[string]interface{}{
				"assigned_to": "",
			})
			b.AssignedTo = ""
		}
		// Try to claim; another worker goroutine may win the race
		if err := e.beadManager.ClaimBead(b.ID, workerID); err != nil {
			continue
		}
		return b
	}
	return nil
}

// executeBead runs a bead through the worker loop. Returns true if the worker
// should back off before claiming the next bead (provider errors, rate limits).
//
// When an AgentManager is configured, the executor looks up a named agent that
// matches the bead's required role and uses that agent's persona and provider.
// The agent's status is updated to "working" during execution and restored to
// "idle" afterward. This makes agent activity visible in the UI.
//
// When no AgentManager is configured (or no matching agent is found), it falls
// back to creating an anonymous worker like the original implementation.
func (e *Executor) executeBead(ctx context.Context, bead *models.Bead, workerID string) (needsBackoff bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[TaskExecutor] PANIC for bead %s: %v", bead.ID, r)
			_ = e.beadManager.UpdateBead(bead.ID, map[string]interface{}{
				"status":      models.BeadStatusOpen,
				"assigned_to": "",
			})
		}
	}()

	providers := e.providerRegistry.ListActive()
	if len(providers) == 0 {
		log.Printf("[TaskExecutor] No active providers, releasing bead %s", bead.ID)
		_ = e.beadManager.UpdateBead(bead.ID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": "",
		})
		return true
	}
	prov := providers[0]

	// Try to find a named agent from the WorkerManager that matches this bead.
	namedAgent := e.findAgentForBead(bead)
	var agentObj *models.Agent
	var agentID string

	if namedAgent != nil {
		agentID = namedAgent.ID
		agentObj = namedAgent

		// Update agent status to working + assign the bead
		e.mu.Lock()
		am := e.agentManager
		e.mu.Unlock()
		if am != nil {
			_ = am.AssignBead(namedAgent.ID, bead.ID)
		}
		log.Printf("[TaskExecutor] Named agent %s (%s) working on bead %s",
			namedAgent.Name, namedAgent.Role, bead.ID)
	} else {
		// Fallback: anonymous worker
		agentID = workerID
		personaName := personaForBead(bead)
		var loadedPersona *models.Persona
		if e.personaManager != nil {
			loadedPersona, _ = e.personaManager.LoadPersona(personaName)
		}
		if loadedPersona == nil {
			loadedPersona = &models.Persona{
				Name:      personaName,
				Character: personas[personaName],
			}
		}
		agentObj = &models.Agent{
			ID:          workerID,
			Name:        personaName,
			PersonaName: personaName,
			ProjectID:   bead.ProjectID,
			ProviderID:  prov.Config.ID,
			Status:      "working",
			Persona:     loadedPersona,
		}
	}

	// Restore agent to idle when done (named agents only)
	if namedAgent != nil {
		defer func() {
			e.mu.Lock()
			am := e.agentManager
			e.mu.Unlock()
			if am != nil {
				_ = am.UpdateAgentStatus(namedAgent.ID, "idle")
			}
		}()
	}

	// Resolve provider — use agent's assigned provider if available
	if agentObj.ProviderID != "" {
		if agentProv, err := e.providerRegistry.Get(agentObj.ProviderID); err == nil {
			prov = agentProv
		}
	}

	w := worker.NewWorker(agentID, agentObj, prov)
	if e.db != nil {
		w.SetDatabase(e.db)
	}

	var proj *models.Project
	if e.projectManager != nil {
		proj, _ = e.projectManager.GetProject(bead.ProjectID)
	}

	task := &worker.Task{
		ID:          fmt.Sprintf("task-%s-%d", bead.ID, time.Now().UnixNano()),
		Description: buildBeadDescription(bead),
		Context:     buildBeadContext(bead, proj),
		BeadID:      bead.ID,
		ProjectID:   bead.ProjectID,
	}

	loopConfig := &worker.LoopConfig{
		MaxIterations: 100,
		Router:        e.actionRouter,
		ActionContext: actions.ActionContext{
			AgentID:   agentID,
			BeadID:    bead.ID,
			ProjectID: bead.ProjectID,
			Model: func() string {
				if prov != nil && prov.Config != nil {
					if prov.Config.SelectedModel != "" {
						return prov.Config.SelectedModel
					}
					return prov.Config.Model
				}
				return ""
			}(),
		},
		LessonsProvider: e.lessonsProvider,
		DB:              e.db,
		TextMode:        !isFullModeCapable(prov),
		OnProgress: func() {
			_ = e.beadManager.UpdateBead(bead.ID, map[string]interface{}{
				"updated_at": time.Now().UTC(),
			})
		},
	}

	result, err := w.ExecuteTaskWithLoop(ctx, task, loopConfig)
	if err != nil {
		log.Printf("[TaskExecutor] ExecuteTaskWithLoop error for bead %s: %v", bead.ID, err)
		e.handleBeadError(bead, err)
		return true
	}

	log.Printf("[TaskExecutor] Bead %s finished: %s (%d iterations)",
		bead.ID, result.TerminalReason, result.Iterations)

	if result.TerminalReason == "completed" {
		_ = e.beadManager.UpdateBead(bead.ID, map[string]interface{}{
			"status":      models.BeadStatusClosed,
			"assigned_to": "",
		})
	} else if result.TerminalReason == "parse_failures" {
		if e.db != nil {
			if _, err := e.db.DB().Exec(
				"DELETE FROM conversation_contexts WHERE bead_id = $1", bead.ID,
			); err != nil {
				log.Printf("[TaskExecutor] Failed to clear conversation for bead %s: %v", bead.ID, err)
			} else {
				log.Printf("[TaskExecutor] Cleared stale conversation history for bead %s (parse_failures)", bead.ID)
			}
		}
		e.handleBeadError(bead, fmt.Errorf("parse_failures: model failed to produce valid actions after %d iterations", result.Iterations))
		return true
	} else {
		_ = e.beadManager.UpdateBead(bead.ID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": "",
		})
	}
	return false
}

// findAgentForBead looks up a named idle agent that matches the bead's required
// role. Uses the org chart to map bead tags/type to the right position.
// Returns nil if no AgentManager is configured or no matching agent is idle.
func (e *Executor) findAgentForBead(bead *models.Bead) *models.Agent {
	e.mu.Lock()
	am := e.agentManager
	e.mu.Unlock()
	if am == nil {
		return nil
	}

	idleAgents := am.GetIdleAgentsByProject(bead.ProjectID)
	if len(idleAgents) == 0 {
		// Try agents not pinned to a project
		idleAgents = am.GetIdleAgentsByProject("")
	}
	if len(idleAgents) == 0 {
		return nil
	}

	targetRole := roleForBead(bead)

	// First pass: exact role match
	for _, ag := range idleAgents {
		if roleMatches(ag.Role, targetRole) {
			return ag
		}
	}

	// Second pass: prefer engineering manager as generalist fallback
	for _, ag := range idleAgents {
		if roleMatches(ag.Role, "engineering-manager") || roleMatches(ag.Role, "Engineering Manager") {
			return ag
		}
	}

	// Third pass: any idle agent (skill portability — any agent can do any work)
	if len(idleAgents) > 0 {
		return idleAgents[0]
	}

	return nil
}

// roleForBead determines which org chart role should handle this bead.
func roleForBead(bead *models.Bead) string {
	// Check bead tags for explicit role hints
	for _, tag := range bead.Tags {
		switch strings.ToLower(tag) {
		case "devops", "infra", "infrastructure", "ci", "cd", "pipeline":
			return "devops-engineer"
		case "review", "pr", "code-review":
			return "code-reviewer"
		case "qa", "test", "testing":
			return "qa-engineer"
		case "docs", "documentation":
			return "documentation-manager"
		case "design", "ui", "ux":
			return "web-designer"
		case "frontend", "web":
			return "web-designer-engineer"
		case "product", "feature", "roadmap":
			return "product-manager"
		case "project", "release", "milestone":
			return "project-manager"
		case "security":
			return "code-reviewer"
		case "remediation", "stuck", "meta":
			return "remediation-specialist"
		}
	}

	// Check bead type
	switch bead.Type {
	case "decision":
		return "decision-maker"
	case "feedback":
		return "product-manager"
	case "delegated":
		// Delegated beads may have a target role in context
		if role := bead.Context["delegate_to_role"]; role != "" {
			return role
		}
	}

	// Default: engineering manager as generalist
	return "engineering-manager"
}

// roleMatches checks if an agent's role matches a target role, handling
// different naming conventions (kebab-case vs Title Case).
func roleMatches(agentRole, targetRole string) bool {
	normalize := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, " ", "-")
		s = strings.ReplaceAll(s, "_", "-")
		return s
	}
	return normalize(agentRole) == normalize(targetRole)
}

// handleBeadError records the error in bead context and detects dispatch loops.
// Context-canceled errors (from loom shutdown) are silently reset.
// Repeated provider/infra errors trigger loop detection and eventual blocking.
func (e *Executor) handleBeadError(bead *models.Bead, execErr error) {
	// Context cancellations are from loom shutdown — silently reset, no history needed.
	if errors.Is(execErr, context.Canceled) || errors.Is(execErr, context.DeadlineExceeded) {
		_ = e.beadManager.UpdateBead(bead.ID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": "",
		})
		return
	}

	// Reload the bead to get fresh context (dispatch_count, error_history).
	fresh, loadErr := e.beadManager.GetBead(bead.ID)
	if loadErr != nil || fresh == nil {
		fresh = bead
	}
	if fresh.Context == nil {
		fresh.Context = map[string]string{}
	}

	// Increment dispatch_count.
	dc := 0
	fmt.Sscanf(fresh.Context["dispatch_count"], "%d", &dc)
	dc++
	fresh.Context["dispatch_count"] = fmt.Sprintf("%d", dc)

	// Append to error_history (capped at 20 entries).
	type errRecord struct {
		Timestamp string `json:"timestamp"`
		Error     string `json:"error"`
		Dispatch  int    `json:"dispatch"`
	}
	var history []errRecord
	if raw := fresh.Context["error_history"]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &history)
	}
	history = append(history, errRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Error:     execErr.Error(),
		Dispatch:  dc,
	})
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	histBytes, _ := json.Marshal(history)
	fresh.Context["error_history"] = string(histBytes)
	fresh.Context["last_run_error"] = execErr.Error()
	fresh.Context["last_run_at"] = time.Now().UTC().Format(time.RFC3339)

	// Run loop detection on the updated bead context.
	ld := loopdetector.NewLoopDetector()
	isStuck, loopReason := ld.IsStuckInLoop(fresh)

	ctxUpdate := map[string]string{
		"dispatch_count": fresh.Context["dispatch_count"],
		"error_history":  fresh.Context["error_history"],
		"last_run_error": fresh.Context["last_run_error"],
		"last_run_at":    fresh.Context["last_run_at"],
		"loop_detected":  fmt.Sprintf("%t", isStuck),
	}
	if isStuck {
		ctxUpdate["loop_detected_reason"] = loopReason
		ctxUpdate["loop_detected_at"] = time.Now().UTC().Format(time.RFC3339)
		log.Printf("[TaskExecutor] Loop detected for bead %s: %s", bead.ID, loopReason)
	}

	newStatus := models.BeadStatusOpen
	if isStuck {
		newStatus = models.BeadStatusBlocked
	}
	_ = e.beadManager.UpdateBead(bead.ID, map[string]interface{}{
		"status":      newStatus,
		"assigned_to": "",
		"context":     ctxUpdate,
	})
}

// personaForBead picks a persona name based on bead tags.
// personaForBead picks a persona name based on bead tags.
func personaForBead(bead *models.Bead) string {
	for _, tag := range bead.Tags {
		switch strings.ToLower(tag) {
		case "devops", "infra", "infrastructure":
			return "devops-engineer"
		case "review", "pr", "code-review":
			return "code-reviewer"
		case "qa", "test", "testing":
			return "qa-engineer"
		case "docs", "documentation":
			return "documentation-manager"
		}
	}
	return "engineering-manager"
}

// buildBeadDescription formats a bead as a task description for the LLM.
func buildBeadDescription(bead *models.Bead) string {
	return fmt.Sprintf("Work on bead %s: %s\n\n%s", bead.ID, bead.Title, bead.Description)
}

// buildBeadContext builds the context string for a bead, including project info,
// architecture reference, and lessons learned from past executions.
func buildBeadContext(bead *models.Bead, proj *models.Project) string {
	var sb strings.Builder

	if proj != nil {
		sb.WriteString(fmt.Sprintf("Project: %s (%s)\nBranch: %s\n", proj.Name, proj.ID, proj.Branch))
		if len(proj.Context) > 0 {
			for k, v := range proj.Context {
				sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
			}
		}
		sb.WriteString("\n")

		workDir := proj.WorkDir
		if workDir == "" {
			workDir = filepath.Join("data", "projects", proj.ID)
		}

		// Architecture reference: gives agents system-level context about how
		// Loom works, bead lifecycle, deadlock patterns, and key invariants.
		// Injected before AGENTS.md so it provides baseline system understanding.
		if archMD := readSystemArchitecture(); archMD != "" {
			sb.WriteString("## Loom System Architecture\n\n")
			sb.WriteString(archMD)
			sb.WriteString("\n\n")
		}

		if agentsMD := readProjectFile(workDir, "AGENTS.md", 4000); agentsMD != "" {
			sb.WriteString("## Project Instructions (AGENTS.md)\n\n")
			sb.WriteString(agentsMD)
			sb.WriteString("\n\n")
		}

		// Project lessons: accumulated lessons from past executions in this project.
		if lessonsMD := readProjectFile(workDir, "LESSONS.md", 3000); lessonsMD != "" {
			sb.WriteString("## Lessons Learned (LESSONS.md)\n\n")
			sb.WriteString(lessonsMD)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString(fmt.Sprintf("Bead: %s (P%d %s)\n", bead.ID, bead.Priority, bead.Type))
	if len(bead.Context) > 0 {
		for k, v := range bead.Context {
			// Skip internal executor fields from the prompt to reduce noise.
			switch k {
			case "dispatch_count", "error_history", "loop_detected",
				"loop_detected_reason", "loop_detected_at", "ralph_blocked_reason":
				continue
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
	}

	sb.WriteString(`
## Instructions

You are an autonomous coding agent. Your job is to MAKE CHANGES, COMMIT, and PUSH.

WORKFLOW:
1. Locate: read AGENTS.md, LESSONS.md, relevant files (iterations 1-3)
2. Change: edit or write files (iterations 4-15)
3. Verify: build and test (iterations 16-18)
4. Land: git_commit, git_push, close_bead/done (iterations 19-21)

CRITICAL RULES:
- You have 100 iterations. Use them.
- ALWAYS git_commit after making changes.
- ALWAYS git_push after committing.
- ALWAYS close_bead or done when the task is complete.
- See "Loom System Architecture" above for deadlock patterns and escape strategies.
`)

	return sb.String()
}

// readSystemArchitecture reads the global LOOM_ARCHITECTURE.md document from the
// loom server's docs directory. Returns empty string if not found.
// This document is injected into every agent's context to provide system-level
// awareness: bead lifecycle, deadlock patterns, agent roles, key invariants.
func readSystemArchitecture() string {
	// Try paths relative to the binary location and common deployment paths.
	candidates := []string{
		"docs/LOOM_ARCHITECTURE.md",
		"/app/docs/LOOM_ARCHITECTURE.md",
		filepath.Join(os.Getenv("HOME"), "docs/LOOM_ARCHITECTURE.md"),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil {
			content := string(data)
			if len(content) > 6000 {
				content = content[:6000] + "\n... (see full docs/LOOM_ARCHITECTURE.md)"
			}
			return content
		}
	}
	return ""
}

// readProjectFile reads a file from a project work directory, capped at maxLen bytes.
func readProjectFile(workDir, filename string, maxLen int) string {
	path := filepath.Join(workDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)
	if len(content) > maxLen {
		content = content[:maxLen] + "\n... (truncated)"
	}
	return content
}

// isFullModeCapable returns true for frontier/large models that support
// the full 60+ action JSON schema. Small/local models use text mode (14 actions).
func isFullModeCapable(prov *provider.RegisteredProvider) bool {
	if prov == nil || prov.Config == nil {
		return false
	}
	name := strings.ToLower(prov.Config.SelectedModel)
	if name == "" {
		name = strings.ToLower(prov.Config.Model)
	}
	for _, prefix := range []string{
		"claude", "anthropic/claude",
		"gpt-4", "gpt-5", "o1", "o3", "o4",
		"gemini-pro", "gemini-1.5", "gemini-2",
	} {
		if strings.HasPrefix(name, prefix) || strings.Contains(name, "/"+prefix) {
			return true
		}
	}
	// Large context window as proxy for frontier model capability
	return prov.Config.ContextWindow > 32768
}
