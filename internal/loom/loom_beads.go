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
func (a *Loom) GetBeadsManager() *beads.Manager {
	return a.beadsManager
}
func (a *Loom) maybeFileReadinessBead(project *models.Project, issues []string, publicKey string) {
	if project == nil || len(issues) == 0 {
		return
	}

	// Dedup key is just the project ID — we don't want multiple open
	// readiness beads for the same project regardless of which specific
	// issues are reported (they tend to fluctuate slightly).
	issueKey := "readiness:" + project.ID
	now := time.Now()
	a.readinessMu.Lock()
	if last, ok := a.readinessFailures[issueKey]; ok && now.Sub(last) < 4*time.Hour {
		a.readinessMu.Unlock()
		return
	}
	a.readinessFailures[issueKey] = now
	a.readinessMu.Unlock()

	// Check if there's already an open/in_progress readiness bead for this project.
	if a.hasOpenReadinessBead(project.ID) {
		return
	}

	description := fmt.Sprintf("Project readiness failed for %s.\n\nIssues:\n- %s", project.ID, strings.Join(issues, "\n- "))
	if publicKey != "" {
		description = fmt.Sprintf("%s\n\nProject SSH public key (register this with your git host):\n%s", description, publicKey)
	}

	bead, err := a.CreateBead(
		fmt.Sprintf("[auto-filed] P3 - Project readiness failed for %s", project.ID),
		description,
		models.BeadPriorityP3,
		"bug",
		project.ID,
	)
	if err != nil {
		log.Printf("failed to auto-file readiness bead for %s: %v", project.ID, err)
		return
	}

	_ = a.beadsManager.UpdateBead(bead.ID, map[string]interface{}{
		"tags": []string{"auto-filed", "readiness", "requires-human-config", "p3"},
	})
}
func (a *Loom) hasOpenReadinessBead(projectID string) bool {
	if a.beadsManager == nil {
		return false
	}
	allBeads, _ := a.beadsManager.GetReadyBeads(projectID)
	for _, b := range allBeads {
		if b == nil || b.ProjectID != projectID {
			continue
		}
		if b.Status != "open" && b.Status != "in_progress" && b.Status != "blocked" {
			continue
		}
		if strings.Contains(strings.ToLower(b.Title), "readiness failed") {
			return true
		}
	}
	return false
}
func normalizeBeadsPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		trimmed = ".beads"
	}

	// Check paths in order of priority
	candidates := []string{
		trimmed,
		// Relative path with dot prefix
		"." + strings.TrimPrefix(trimmed, "/"),
		// Fallback
		".beads",
	}

	for _, candidate := range candidates {
		if beadsPathExists(candidate) {
			return candidate
		}
	}
	return trimmed
}
func beadsPathExists(path string) bool {
	if path == "" {
		return false
	}
	issuesPath := filepath.Join(path, "issues.jsonl")
	if _, err := os.Stat(issuesPath); err == nil {
		return true
	}
	beadsDir := filepath.Join(path, "beads")
	if _, err := os.Stat(beadsDir); err == nil {
		return true
	}
	return false
}
func (a *Loom) CreateBead(title, description string, priority models.BeadPriority, beadType, projectID string) (*models.Bead, error) {
	// Verify project exists
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	bead, err := a.beadsManager.CreateBead(title, description, priority, beadType, projectID)
	if err != nil {
		return nil, err
	}

	// Auto-assignment removed: TaskExecutor claims beads directly via ClaimBead.
	// Pre-assigning a bead prevents the executor from claiming it.

	if a.eventBus != nil {
		_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadCreated, bead.ID, projectID, map[string]interface{}{
			"title":       title,
			"type":        beadType,
			"priority":    priority,
			"assigned_to": bead.AssignedTo,
		})
	}

	// Auto-approve low-risk code fix proposals to enable full autonomy
	if beadType == "decision" && strings.Contains(strings.ToLower(title), "code fix approval") {
		go a.tryAutoApproveCodeFix(bead)
	}

	return bead, nil
}
func (a *Loom) CloseBead(beadID, reason string) error {
	bead, err := a.beadsManager.GetBead(beadID)
	if err != nil {
		return fmt.Errorf("bead not found: %w", err)
	}

	updates := map[string]interface{}{
		"status": models.BeadStatusClosed,
	}
	if reason != "" {
		ctx := bead.Context
		if ctx == nil {
			ctx = make(map[string]string)
		}
		ctx["close_reason"] = reason
		updates["context"] = ctx
	}

	if err := a.beadsManager.UpdateBead(beadID, updates); err != nil {
		return fmt.Errorf("failed to close bead: %w", err)
	}

	if a.eventBus != nil {
		_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, beadID, bead.ProjectID, map[string]interface{}{
			"status": string(models.BeadStatusClosed),
			"reason": reason,
		})
	}

	// Clean up agent worktree if one was allocated for this bead.
	if bead.ProjectID != "" {
		wtManager := gitops.NewGitWorktreeManager(a.config.Git.ProjectKeyDir)
		if err := wtManager.CleanupAgentWorktree(bead.ProjectID, beadID); err != nil {
			log.Printf("[Loom] Worktree cleanup for bead %s failed (non-fatal): %v", beadID, err)
		}
	}

	// Auto-create apply-fix bead if this was an approved code fix proposal
	if strings.Contains(strings.ToLower(bead.Title), "code fix approval") &&
		bead.Type == "decision" &&
		strings.Contains(strings.ToLower(reason), "approve") {

		if err := a.createApplyFixBead(bead, reason); err != nil {
			log.Printf("[AutoFix] Failed to create apply-fix bead for %s: %v", beadID, err)
			// Don't fail the close operation if apply-fix creation fails
		}
	}

	return nil
}
func (a *Loom) createApplyFixBead(approvalBead *models.Bead, closeReason string) error {
	// Extract original bug ID from approval bead description
	originalBugID := extractOriginalBugID(approvalBead.Description)
	if originalBugID == "" {
		return fmt.Errorf("could not extract original bug ID from approval bead")
	}

	// Get the agent who created the proposal (from context or assigned_to)
	agentID := ""
	if approvalBead.Context != nil {
		agentID = approvalBead.Context["agent_id"]
	}
	if agentID == "" && approvalBead.AssignedTo != "" {
		agentID = approvalBead.AssignedTo
	}

	projectID := approvalBead.ProjectID
	if projectID == "" {
		projectID = a.config.GetSelfProjectID()
	}

	// Create apply-fix bead
	title := fmt.Sprintf("[apply-fix] Apply approved patch from %s", approvalBead.ID)

	description := fmt.Sprintf(`## Apply Approved Code Fix

**Approval Bead:** %s
**Original Bug:** %s
**Approved By:** CEO
**Approved At:** %s
**Approval Reason:** %s

### Instructions

1. Read the approved fix proposal from bead %s
2. Extract the patch or code changes from the proposal
3. Apply the changes using write_file or apply_patch action
4. Verify the fix (compile/test if applicable)
5. Update cache versions if needed (for frontend changes)
6. Close this bead and the original bug bead %s
7. Add comment to bug bead: "Fixed by applying approved patch from %s"

### Approved Proposal

%s

### Important Notes

- This fix has been reviewed and approved by the CEO
- Apply the changes exactly as specified in the proposal
- Test thoroughly after applying
- Report any issues or unexpected errors immediately
- If hot-reload is enabled, verify the fix works after automatic browser refresh
`,
		approvalBead.ID,
		originalBugID,
		time.Now().Format(time.RFC3339),
		closeReason,
		approvalBead.ID,
		originalBugID,
		approvalBead.ID,
		approvalBead.Description,
	)

	// Create the bead
	bead, err := a.CreateBead(title, description, models.BeadPriority(1), "task", projectID)
	if err != nil {
		return fmt.Errorf("failed to create apply-fix bead: %w", err)
	}

	// Update with tags, assignment, and context
	tags := []string{"apply-fix", "auto-created", "code-fix"}
	ctx := map[string]string{
		"approval_bead_id": approvalBead.ID,
		"original_bug_id":  originalBugID,
		"fix_type":         "code-fix",
		"created_by":       "auto_fix_system",
	}

	updates := map[string]interface{}{
		"tags":    tags,
		"context": ctx,
	}

	// Assign to the agent who created the proposal, if available
	if agentID != "" {
		updates["assigned_to"] = agentID
	}

	if err := a.beadsManager.UpdateBead(bead.ID, updates); err != nil {
		log.Printf("[AutoFix] Failed to update apply-fix bead %s: %v", bead.ID, err)
		// Don't fail - bead is created, just missing some metadata
	}

	log.Printf("[AutoFix] Created apply-fix bead %s for approved proposal %s (original bug: %s)",
		bead.ID, approvalBead.ID, originalBugID)

	return nil
}
func (a *Loom) CreateDecisionBead(question, parentBeadID, requesterID string, options []string, recommendation string, priority models.BeadPriority, projectID string) (*models.DecisionBead, error) {
	// Verify requester exists (agent or user/system)
	if requesterID != "system" && !strings.HasPrefix(requesterID, "user-") {
		if _, err := a.agentManager.GetAgent(requesterID); err != nil {
			return nil, fmt.Errorf("requester agent not found: %w", err)
		}
	}

	// Create decision
	decision, err := a.decisionManager.CreateDecision(question, parentBeadID, requesterID, options, recommendation, priority, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create decision: %w", err)
	}

	// Block parent bead on this decision
	if parentBeadID != "" {
		if err := a.beadsManager.AddDependency(parentBeadID, decision.ID, "blocks"); err != nil {
			return nil, fmt.Errorf("failed to add blocking dependency: %w", err)
		}
	}

	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeDecisionCreated,
			Source:    "decision-manager",
			ProjectID: projectID,
			Data: map[string]interface{}{
				"decision_id":  decision.ID,
				"question":     question,
				"requester_id": requesterID,
			},
		})
	}

	return decision, nil
}
func (a *Loom) EscalateBeadToCEO(beadID, reason, returnedTo string) (*models.DecisionBead, error) {
	b, err := a.beadsManager.GetBead(beadID)
	if err != nil {
		return nil, fmt.Errorf("bead not found: %w", err)
	}
	if returnedTo == "" {
		returnedTo = b.AssignedTo
	}

	question := fmt.Sprintf("CEO decision required for bead %s (%s).\n\nReason: %s\n\nChoose: approve | deny | needs_more_info", b.ID, b.Title, reason)
	decision, err := a.decisionManager.CreateDecision(question, beadID, "system", []string{"approve", "deny", "needs_more_info"}, "", models.BeadPriorityP0, b.ProjectID)
	if err != nil {
		return nil, err
	}
	if decision.Context == nil {
		decision.Context = make(map[string]string)
	}
	decision.Context["escalated_to"] = "ceo"
	decision.Context["returned_to"] = returnedTo
	decision.Context["escalation_reason"] = reason

	_, _ = a.UpdateBead(beadID, map[string]interface{}{
		"priority": models.BeadPriorityP0,
		"context": map[string]string{
			"escalated_to_ceo_at":          time.Now().UTC().Format(time.RFC3339),
			"escalated_to_ceo_reason":      reason,
			"escalated_to_ceo_decision_id": decision.ID,
		},
	})

	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeDecisionCreated,
			Source:    "ceo-escalation",
			ProjectID: b.ProjectID,
			Data: map[string]interface{}{
				"decision_id": decision.ID,
				"bead_id":     beadID,
				"reason":      reason,
			},
		})
	}

	return decision, nil
}
func (a *Loom) ClaimBead(beadID, agentID string) error {
	// Verify agent exists
	if _, err := a.agentManager.GetAgent(agentID); err != nil {
		observability.Error("bead.claim", map[string]interface{}{
			"agent_id": agentID,
			"bead_id":  beadID,
		}, err)
		return fmt.Errorf("agent not found: %w", err)
	}

	// Claim the bead
	if err := a.beadsManager.ClaimBead(beadID, agentID); err != nil {
		observability.Error("bead.claim", map[string]interface{}{
			"agent_id": agentID,
			"bead_id":  beadID,
		}, err)
		return fmt.Errorf("failed to claim bead: %w", err)
	}

	// Update agent status
	if err := a.agentManager.AssignBead(agentID, beadID); err != nil {
		observability.Error("agent.assign_bead", map[string]interface{}{
			"agent_id": agentID,
			"bead_id":  beadID,
		}, err)
		return fmt.Errorf("failed to assign bead to agent: %w", err)
	}

	if a.eventBus != nil {
		projectID := ""
		if b, err := a.beadsManager.GetBead(beadID); err == nil && b != nil {
			projectID = b.ProjectID
		}
		_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadAssigned, beadID, projectID, map[string]interface{}{
			"assigned_to": agentID,
		})
		_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, beadID, projectID, map[string]interface{}{
			"status": string(models.BeadStatusInProgress),
		})
	}

	projectID := ""
	if b, err := a.beadsManager.GetBead(beadID); err == nil && b != nil {
		projectID = b.ProjectID
	}
	observability.Info("bead.claim", map[string]interface{}{
		"agent_id":   agentID,
		"bead_id":    beadID,
		"project_id": projectID,
		"status":     "claimed",
	})

	return nil
}
func (a *Loom) UpdateBead(beadID string, updates map[string]interface{}) (*models.Bead, error) {
	if err := a.beadsManager.UpdateBead(beadID, updates); err != nil {
		return nil, err
	}

	bead, err := a.beadsManager.GetBead(beadID)
	if err != nil {
		return nil, err
	}

	if a.eventBus != nil {
		if status, ok := updates["status"].(models.BeadStatus); ok {
			_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, beadID, bead.ProjectID, map[string]interface{}{
				"status": string(status),
			})
			if status == models.BeadStatusClosed {
				_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadCompleted, beadID, bead.ProjectID, map[string]interface{}{})
			}
		}
		if assignedTo, ok := updates["assigned_to"].(string); ok && assignedTo != "" {
			_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadAssigned, beadID, bead.ProjectID, map[string]interface{}{
				"assigned_to": assignedTo,
			})
		}
	}

	return bead, nil
}
func (a *Loom) GetReadyBeads(projectID string) ([]*models.Bead, error) {
	return a.beadsManager.GetReadyBeads(projectID)
}
func (a *Loom) GetBead(beadID string) (*models.Bead, error) {
	return a.beadsManager.GetBead(beadID)
}
func (a *Loom) GetBeadConversation(beadID string) ([]models.ChatMessage, error) {
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}

	ctx, err := a.database.GetConversationContextByBeadID(beadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	return ctx.Messages, nil
}
func (a *Loom) GetBeadsWithUpcomingDeadlines(withinDays int) ([]motivation.BeadDeadlineInfo, error) {
	if a.beadsManager == nil {
		return nil, fmt.Errorf("beads manager not available")
	}
	// TODO: Implement deadline checking
	return []motivation.BeadDeadlineInfo{}, nil
}
func (a *Loom) GetOverdueBeads() ([]motivation.BeadDeadlineInfo, error) {
	if a.beadsManager == nil {
		return nil, fmt.Errorf("beads manager not available")
	}
	// TODO: Implement overdue checking
	return []motivation.BeadDeadlineInfo{}, nil
}
func (a *Loom) GetBeadsByStatus(status string) ([]string, error) {
	if a.beadsManager == nil {
		return nil, fmt.Errorf("beads manager not available")
	}
	// TODO: Implement status filtering
	return []string{}, nil
}
func (a *Loom) CreateStimulusBead(motivation *motivation.Motivation, triggerData map[string]interface{}) (string, error) {
	if a.beadsManager == nil {
		return "", fmt.Errorf("beads manager not available")
	}
	// Create a bead with the motivation as context
	title := fmt.Sprintf("Stimulus: %s", motivation.Title)
	description := fmt.Sprintf("Triggered by motivation: %s\nTrigger data: %v", motivation.Title, triggerData)
	// TODO: Use proper bead creation API
	return "", nil
}
