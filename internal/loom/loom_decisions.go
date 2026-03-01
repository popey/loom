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

func (a *Loom) GetDecisionManager() *decision.Manager {
	return a.decisionManager
}
func (a *Loom) MakeDecision(decisionID, deciderID, decisionText, rationale string) error {
	// Verify decider exists (could be agent or user)
	// For users, we'll allow any decider ID starting with "user-"
	if !strings.HasPrefix(deciderID, "user-") {
		if _, err := a.agentManager.GetAgent(deciderID); err != nil {
			return fmt.Errorf("decider not found: %w", err)
		}
	}

	// Make decision
	if err := a.decisionManager.MakeDecision(decisionID, deciderID, decisionText, rationale); err != nil {
		return fmt.Errorf("failed to make decision: %w", err)
	}

	// Unblock dependent beads
	if err := a.UnblockDependents(decisionID); err != nil {
		return fmt.Errorf("failed to unblock dependents: %w", err)
	}

	if a.eventBus != nil {
		if d, err := a.decisionManager.GetDecision(decisionID); err == nil && d != nil {
			_ = a.eventBus.Publish(&eventbus.Event{
				Type:      eventbus.EventTypeDecisionResolved,
				Source:    "decision-manager",
				ProjectID: d.ProjectID,
				Data: map[string]interface{}{
					"decision_id": decisionID,
					"decision":    decisionText,
					"decider_id":  deciderID,
				},
			})
		}
	}

	_ = a.applyCEODecisionToParent(decisionID)

	return nil
}
func (a *Loom) applyCEODecisionToParent(decisionID string) error {
	d, err := a.decisionManager.GetDecision(decisionID)
	if err != nil || d == nil || d.Context == nil {
		return nil
	}
	if d.Context["escalated_to"] != "ceo" {
		return nil
	}
	parentID := d.Parent
	if parentID == "" {
		return nil
	}

	decision := strings.ToLower(strings.TrimSpace(d.Decision))
	switch decision {
	case "approve":
		_, _ = a.UpdateBead(parentID, map[string]interface{}{"status": models.BeadStatusClosed})
	case "deny":
		// Reassign to default triage agent instead of leaving unassigned
		denyAssignee := ""
		if parentBead, err := a.beadsManager.GetBead(parentID); err == nil {
			denyAssignee = a.findDefaultAssignee(parentBead.ProjectID)
		}
		_, _ = a.UpdateBead(parentID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": denyAssignee,
			"context": map[string]string{
				"ceo_denied_at":      time.Now().UTC().Format(time.RFC3339),
				"ceo_comment":        d.Rationale,
				"reassigned_to_role": "default-triage",
			},
		})
	case "needs_more_info":
		returnedTo := d.Context["returned_to"]
		_, _ = a.UpdateBead(parentID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": returnedTo,
			"context": map[string]string{
				"redispatch_requested":   "true",
				"ceo_needs_more_info_at": time.Now().UTC().Format(time.RFC3339),
				"ceo_comment":            d.Rationale,
			},
		})
	}

	return nil
}
func (a *Loom) GetPendingDecisions() ([]string, error) {
	if a.consensusManager == nil {
		return nil, fmt.Errorf("consensus manager not available")
	}
	// TODO: Implement pending decision retrieval
	return []string{}, nil
}
