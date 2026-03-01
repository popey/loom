package loom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jordanhubbard/loom/internal/actions"
	"github.com/jordanhubbard/loom/internal/activity"
	"github.com/jordanhubbard/loom/internal/agent"
	"github.com/jordanhubbard/loom/internal/analytics"
	"github.com/jordanhubbard/loom/internal/beads"
	"github.com/jordanhubbard/loom/internal/collaboration"
	"github.com/jordanhubbard/loom/internal/comments"
	"github.com/jordanhubbard/loom/internal/consensus"
	"github.com/jordanhubbard/loom/internal/containers"
	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/decision"
	"github.com/jordanhubbard/loom/internal/eventbus"
	"github.com/jordanhubbard/loom/internal/executor"
	"github.com/jordanhubbard/loom/internal/files"
	"github.com/jordanhubbard/loom/internal/gitops"
	"github.com/jordanhubbard/loom/internal/keymanager"
	"github.com/jordanhubbard/loom/internal/logging"
	"github.com/jordanhubbard/loom/internal/memory"
	"github.com/jordanhubbard/loom/internal/messagebus"
	"github.com/jordanhubbard/loom/internal/meetings"
	"github.com/jordanhubbard/loom/internal/metrics"
	"github.com/jordanhubbard/loom/internal/modelcatalog"
	internalmodels "github.com/jordanhubbard/loom/internal/models"
	"github.com/jordanhubbard/loom/internal/motivation"
	"github.com/jordanhubbard/loom/internal/notifications"
	"github.com/jordanhubbard/loom/internal/observability"
	"github.com/jordanhubbard/loom/internal/openclaw"
	"github.com/jordanhubbard/loom/internal/orchestrator"
	"github.com/jordanhubbard/loom/internal/orgchart"
	"github.com/jordanhubbard/loom/internal/patterns"
	"github.com/jordanhubbard/loom/internal/persona"
	"github.com/jordanhubbard/loom/internal/project"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/internal/ralph"
	"github.com/jordanhubbard/loom/internal/statusboard"
	"github.com/jordanhubbard/loom/internal/swarm"
	"github.com/jordanhubbard/loom/internal/taskexecutor"
	"github.com/jordanhubbard/loom/internal/workflow"
	"github.com/jordanhubbard/loom/pkg/config"
	"github.com/jordanhubbard/loom/pkg/connectors"
	"github.com/jordanhubbard/loom/pkg/models"
)

const readinessCacheTTL = 2 * time.Minute

type projectReadinessState struct {
	ready     bool
	issues    []string
	checkedAt time.Time
}

// Loom is the main orchestrator
type Loom struct {
	config                *config.Config
	agentManager          *agent.WorkerManager
	actionRouter          *actions.Router
	projectManager        *project.Manager
	personaManager        *persona.Manager
	beadsManager          *beads.Manager
	decisionManager       *decision.Manager
	fileLockManager       *FileLockManager
	orgChartManager       *orgchart.Manager
	providerRegistry      *provider.Registry
	database              *database.Database
	eventBus              *eventbus.EventBus
	modelCatalog          *modelcatalog.Catalog
	gitopsManager         *gitops.Manager
	shellExecutor         *executor.ShellExecutor
	logManager            *logging.Manager
	activityManager       *activity.Manager
	notificationManager   *notifications.Manager
	commentsManager       *comments.Manager
	collaborationStore    *collaboration.ContextStore
	consensusManager      *consensus.DecisionManager
	meetingsManager       *meetings.Manager
	motivationRegistry    *motivation.Registry
	motivationEngine      *motivation.Engine
	idleDetector          *motivation.IdleDetector
	workflowEngine        *workflow.Engine
	patternManager        *patterns.Manager
	metrics               *metrics.Metrics
	keyManager            *keymanager.KeyManager
	doltCoordinator       *beads.DoltCoordinator
	openclawClient        *openclaw.Client
	openclawBridge        *openclaw.Bridge
	containerOrchestrator *containers.Orchestrator
	connectorManager      *connectors.Manager
	memoryManager         *memory.MemoryManager
	messageBus            interface{}
	bridge                *messagebus.BridgedMessageBus
	pdaOrchestrator       *orchestrator.PDAOrchestrator
	swarmManager          *swarm.Manager
	swarmFederation       *swarm.Federation
	taskExecutor          *taskexecutor.Executor
	statusBoard           *statusboard.Board
	readinessMu           sync.Mutex
	readinessCache        map[string]projectReadinessState
	readinessFailures     map[string]time.Time
	shutdownOnce          sync.Once
	startedAt             time.Time
}

func (a *Loom) kickstartOpenBeads(ctx context.Context) {
	projects := a.projectManager.ListProjects()
	if len(projects) == 0 {
		return
	}

	var totalKickstarted int
	for _, p := range projects {
		if p == nil || p.ID == "" {
			continue
		}

		beadsList, err := a.beadsManager.GetReadyBeads(p.ID)
		if err != nil {
			continue
		}

		for _, b := range beadsList {
			if b == nil {
				continue
			}
			// Skip decision beads - they require human/CEO input
			if b.Type == "decision" {
				continue
			}
			// Skip beads that are already in progress with an assigned agent
			if b.Status == models.BeadStatusInProgress && b.AssignedTo != "" {
				continue
			}

			// Publish event to signal work is available
			if a.eventBus != nil {
				_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadCreated, b.ID, p.ID, map[string]interface{}{
					"title":       b.Title,
					"type":        b.Type,
					"priority":    b.Priority,
					"kickstarted": true,
				})
			}

			totalKickstarted++
		}
	}

	if totalKickstarted > 0 {
		fmt.Printf("Kickstarted %d open bead(s) across %d project(s)\n", totalKickstarted, len(projects))
	}
}
	if a.beadsManager == nil {
		return 0
	}

	inProgressBeads, err := a.beadsManager.ListBeads(map[string]interface{}{
		"status": models.BeadStatusInProgress,
	})
	if err != nil {
		log.Printf("[Loom] resetZombieBeads: could not list in-progress beads: %v", err)
		return 0
	}

	// Build a set of known live agent IDs so we can detect beads assigned to
	// named agents that no longer exist or are permanently idle.
	liveAgentIDs := make(map[string]bool)
	if a.agentManager != nil {
		for _, ag := range a.agentManager.ListAgents() {
			if ag != nil {
				liveAgentIDs[ag.ID] = true
			}
		}
	}

	count := 0
	for _, b := range inProgressBeads {
		if b == nil || b.AssignedTo == "" {
			continue
		}
		isZombie := false
		if strings.HasPrefix(b.AssignedTo, "exec-") {
			// Ephemeral goroutine ID — never survives restart
			isZombie = true
		} else if !liveAgentIDs[b.AssignedTo] {
			// Named agent ID that is not registered in the current run
			isZombie = true
		}
		if !isZombie {
			continue
		}
		if err := a.beadsManager.UpdateBead(b.ID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": "",
		}); err != nil {
			log.Printf("[Loom] resetZombieBeads: could not reset bead %s: %v", b.ID, err)
			continue
		}
		log.Printf("[Loom] Recovered zombie bead %s [%s] (was held by stale assignee %s)",
			b.ID, b.Title, b.AssignedTo)
		count++
	}
	return count
}
func (a *Loom) retireInactiveAgents(threshold time.Duration) {
	if a.agentManager == nil || a.orgChartManager == nil {
		return
	}

	// Build the set of required roles so we never auto-retire them.
	required := make(map[string]struct{})
	for _, pos := range models.DefaultOrgChartPositions() {
		if pos.Required {
			required[strings.ToLower(pos.RoleName)] = struct{}{}
		}
	}

	now := time.Now()
	for _, ag := range a.agentManager.ListAgents() {
		if ag == nil {
			continue
		}
		role := strings.ToLower(ag.Role)
		if role == "" {
			role = strings.ToLower(roleFromPersonaName(ag.PersonaName))
		}
		// Never retire required roles.
		if _, isRequired := required[role]; isRequired {
			continue
		}
		// Skip agents that have been active recently.
		if now.Sub(ag.LastActive) < threshold {
			continue
		}
		// Skip agents that are currently working.
		if ag.Status == "working" {
			continue
		}
		positions := a.orgChartManager.GetPositionsForAgent(ag.ProjectID, ag.ID)
		if len(positions) == 0 {
			continue
		}
		if err := a.orgChartManager.RemoveAgentFromAll(ag.ProjectID, ag.ID); err != nil {
			log.Printf("[Maintenance] Failed to retire agent %s from org chart: %v", ag.ID, err)
			continue
		}
		log.Printf("[Maintenance] Retired inactive agent %s (role: %s, last active: %s)",
			ag.ID, role, ag.LastActive.Format("2006-01-02"))
	}
}
func (a *Loom) resetInconsistentAgents() int {
	if a.agentManager == nil || a.database == nil {
		return 0
	}

	agents, err := a.database.ListAgents()
	if err != nil {
		return 0
	}

	count := 0
	workingCount := 0
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		if agent.Status == "working" {
			workingCount++
			log.Printf("[DispatchLoop] Found working agent %s (currentBead=%q)", agent.ID, agent.CurrentBead)
		}
		if agent.Status != "working" {
			continue
		}
		shouldReset := agent.CurrentBead == ""
		if !shouldReset {
			// Also reset if bead is closed or no longer exists
			bead, beadErr := a.beadsManager.GetBead(agent.CurrentBead)
			if beadErr != nil || bead == nil || bead.Status == models.BeadStatusClosed {
				shouldReset = true
				log.Printf("[DispatchLoop] Agent %s stuck on closed/missing bead %q", agent.ID, agent.CurrentBead)
			}
		}
		if !shouldReset {
			// Also reset agents whose last_active is stale (worker goroutine died on restart)
			if staleness := time.Since(agent.LastActive); staleness > 10*time.Minute {
				shouldReset = true
				log.Printf("[DispatchLoop] Agent %s stale (last_active %v ago, bead=%q)", agent.ID, staleness.Round(time.Second), agent.CurrentBead)
			}
		}
		if shouldReset {
			prevBead := agent.CurrentBead
			agent.Status = "idle"
			agent.CurrentBead = ""
			if err := a.database.UpsertAgent(agent); err == nil {
				log.Printf("[DispatchLoop] Reset inconsistent agent %s (was working on %q)", agent.ID, prevBead)
				count++
			}
		}
	}
	return count
}
