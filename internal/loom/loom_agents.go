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

func (a *Loom) GetAgentManager() *agent.WorkerManager {
	return a.agentManager
}
func (a *Loom) ensureDefaultAgents(ctx context.Context, projectID string) error {
	return a.ensureOrgChart(ctx, projectID)
}
func (a *Loom) EnsureDefaultAgents(ctx context.Context, projectID string) error {
	return a.ensureDefaultAgents(ctx, projectID)
}
func formatAgentName(roleName, personaType string) string {
	// Convert kebab-case to Title Case
	words := strings.Split(roleName, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	titleRole := strings.Join(words, " ")
	// Capitalize acronyms like CEO, CFO
	titleRole = capitalizeAcronyms(titleRole)
	return fmt.Sprintf("%s (%s)", titleRole, personaType)
}
func (a *Loom) CloneAgentPersona(ctx context.Context, agentID, newPersonaName, newAgentName, sourcePersona string, replace bool) (*models.Agent, error) {
	agent, err := a.agentManager.GetAgent(agentID)
	if err != nil {
		return nil, err
	}
	if newPersonaName == "" {
		return nil, errors.New("new persona name is required")
	}

	roleName := ""
	if strings.HasPrefix(agent.PersonaName, "default/") {
		roleName = strings.TrimPrefix(agent.PersonaName, "default/")
	}
	if roleName == "" {
		roleName = path.Base(agent.PersonaName)
	}

	if sourcePersona == "" {
		sourcePersona = fmt.Sprintf("default/%s", roleName)
	}

	clonedPersona := fmt.Sprintf("projects/%s/%s/%s", agent.ProjectID, roleName, newPersonaName)
	_, err = a.personaManager.ClonePersona(sourcePersona, clonedPersona)
	if err != nil {
		return nil, err
	}

	if newAgentName == "" {
		newAgentName = fmt.Sprintf("%s-%s", roleName, newPersonaName)
	}
	newAgent, err := a.SpawnAgent(ctx, newAgentName, clonedPersona, agent.ProjectID, agent.ProviderID)
	if err != nil {
		return nil, err
	}

	if replace {
		_ = a.StopAgent(ctx, agent.ID)
	}

	return newAgent, nil
}
func (a *Loom) CreateAgent(ctx context.Context, name, personaName, projectID, role string) (*models.Agent, error) {
	// Load persona
	persona, err := a.personaManager.LoadPersona(personaName)
	if err != nil {
		return nil, fmt.Errorf("failed to load persona: %w", err)
	}

	// Verify project exists
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Create agent record without a worker
	agent, err := a.agentManager.CreateAgent(ctx, name, personaName, projectID, role, persona)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Add agent to project
	if err := a.projectManager.AddAgentToProject(projectID, agent.ID); err != nil {
		return nil, fmt.Errorf("failed to add agent to project: %w", err)
	}

	// Persist agent to the configuration database
	if a.database != nil {
		if err := a.database.UpsertAgent(agent); err != nil {
			log.Printf("Warning: Failed to persist agent %s to database: %v", agent.ID, err)
		} else {
			log.Printf("Persisted agent %s (%s) to database with status: %s", agent.ID, agent.Name, agent.Status)
		}
	}

	return agent, nil
}
func (a *Loom) SpawnAgent(ctx context.Context, name, personaName, projectID string, providerID string) (*models.Agent, error) {
	// Load persona
	persona, err := a.personaManager.LoadPersona(personaName)
	if err != nil {
		return nil, fmt.Errorf("failed to load persona: %w", err)
	}

	// Verify project exists
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// If no provider specified, pick the first registered provider.
	if providerID == "" {
		providers := a.providerRegistry.ListActive()
		if len(providers) == 0 {
			return nil, fmt.Errorf("no active providers registered")
		}
		providerID = providers[0].Config.ID
	}

	// Spawn agent + worker
	agent, err := a.agentManager.SpawnAgentWorker(ctx, name, personaName, projectID, providerID, persona)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn agent: %w", err)
	}

	// Add agent to project
	if err := a.projectManager.AddAgentToProject(projectID, agent.ID); err != nil {
		return nil, fmt.Errorf("failed to add agent to project: %w", err)
	}

	// Persist agent assignment to the configuration database.
	if a.database != nil {
		_ = a.database.UpsertAgent(agent)
	}

	return agent, nil
}
func (a *Loom) StopAgent(ctx context.Context, agentID string) error {
	ag, err := a.agentManager.GetAgent(agentID)
	if err != nil {
		return err
	}

	if err := a.agentManager.StopAgent(agentID); err != nil {
		return err
	}
	_ = a.fileLockManager.ReleaseAgentLocks(agentID)
	_ = a.projectManager.RemoveAgentFromProject(ag.ProjectID, ag.ID)
	_ = a.orgChartManager.RemoveAgentFromAll(ag.ProjectID, agentID)
	a.PersistProject(ag.ProjectID)
	if a.database != nil {
		_ = a.database.DeleteAgent(agentID)
	}
	return nil
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
func (a *Loom) ConsultAgent(ctx context.Context, fromAgentID, toAgentID, toRole, question string) (string, error) {
	if a.agentManager == nil {
		return "", fmt.Errorf("agent manager not available")
	}
	
	// Find the target agent
	var targetAgent *agent.Agent
	if toAgentID != "" {
		targetAgent = a.agentManager.GetAgent(toAgentID)
	} else if toRole != "" {
		targetAgent = a.agentManager.GetAgentByRole(toRole)
	}
	
	if targetAgent == nil {
		return "", fmt.Errorf("target agent not found")
	}
	
	// Send the consultation prompt to the agent
	// This will be handled by the agent's message processing loop
	return targetAgent.ConsultWithPrompt(ctx, question)
}
func (a *Loom) GetIdleAgents() ([]string, error) {
	if a.idleDetector == nil {
		return nil, fmt.Errorf("idle detector not available")
	}
	return a.idleDetector.GetIdleAgents(), nil
}
func (a *Loom) GetAgentsByRole(role string) ([]string, error) {
	if a.agentManager == nil {
		return nil, fmt.Errorf("agent manager not available")
	}
	// TODO: Implement role-based agent retrieval
	return []string{}, nil
}
func (a *Loom) WakeAgent(agentID string, motivation *motivation.Motivation) error {
	if a.agentManager == nil {
		return fmt.Errorf("agent manager not available")
	}
	// TODO: Implement agent wake-up
	return nil
}
func (a *Loom) WakeAgentsByRole(role string, motivation *motivation.Motivation) error {
	if a.agentManager == nil {
		return fmt.Errorf("agent manager not available")
	}
	// TODO: Implement role-based agent wake-up
	return nil
}
