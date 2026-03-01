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

	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("message is required")
	}
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}

	// Extract persona hint and clean message if "persona: message" format is used
	personaHint, cleanMessage := extractPersonaFromMessage(message)

	// Create a P0 bead for this CEO query
	beadTitle := "CEO Query"
	if personaHint != "" {
		beadTitle = fmt.Sprintf("CEO Query for %s", personaHint)
	}

	// Truncate message for title if it's short
	if len(cleanMessage) < 80 {
		beadTitle = fmt.Sprintf("CEO: %s", cleanMessage)
	}

	bead, err := a.beadsManager.CreateBead(beadTitle, cleanMessage, models.BeadPriorityP0, "task", a.config.GetSelfProjectID())
	if err != nil {
		// If bead creation fails, continue anyway but log it
		log.Printf("Warning: Failed to create CEO query bead: %v", err)
	}

	var beadID string
	if bead != nil {
		beadID = bead.ID

		// If persona hint was provided, add it to the bead description
		if personaHint != "" {
			updatedDesc := fmt.Sprintf("Persona: %s\n\n%s", personaHint, cleanMessage)
			_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
				"description": updatedDesc,
			})
		}

		// Add CEO context
		_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
			"context": map[string]string{
				"source":     "ceo-repl",
				"created_by": "ceo",
			},
		})
	}

	providerRecord, err := a.selectBestProviderForRepl()
	if err != nil {
		return nil, err
	}

	systemPrompt := a.buildLoomPersonaPrompt()

	regProvider, err := a.providerRegistry.Get(providerRecord.ID)
	if err != nil {
		return nil, fmt.Errorf("provider %s not found in registry: %w", providerRecord.ID, err)
	}
	if regProvider.Protocol == nil {
		return nil, fmt.Errorf("provider %s has no protocol configured", providerRecord.ID)
	}

	model := providerRecord.SelectedModel
	if model == "" {
		model = providerRecord.Model
	}
	if model == "" {
		model = providerRecord.ConfiguredModel
	}

	req := &provider.ChatCompletionRequest{
		Model: model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: cleanMessage},
		},
		Temperature: 0.2,
		MaxTokens:   1200,
	}

	queryStart := time.Now()
	resp, err := regProvider.Protocol.CreateChatCompletion(ctx, req)
	latencyMs := time.Since(queryStart).Milliseconds()
	if err != nil {
		// Update bead with error if it was created
		if beadID != "" {
			_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
				"context": map[string]string{
					"source":     "ceo-repl",
					"created_by": "ceo",
					"error":      err.Error(),
				},
			})
		}
		return nil, err
	}

	responseText := ""
	if len(resp.Choices) > 0 {
		responseText = resp.Choices[0].Message.Content
	}
	tokensUsed := resp.Usage.TotalTokens
	responseModel := resp.Model

	// Enforce strict JSON action output and execute actions
	var actionResults []actions.Result
	if a.actionRouter != nil {
		actx := actions.ActionContext{
			AgentID:   "ceo",
			BeadID:    beadID,
			ProjectID: a.config.GetSelfProjectID(),
		}
		env, parseErr := actions.DecodeLenient([]byte(responseText))
		if parseErr != nil {
			actionResult := a.actionRouter.AutoFileParseFailure(ctx, actx, parseErr, responseText)
			actionResults = []actions.Result{actionResult}
		} else {
			actionResults, _ = a.actionRouter.Execute(ctx, env, actx)
		}
	}

	// Update bead with response
	if beadID != "" {
		actionsJSON, _ := json.Marshal(actionResults)
		_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
			"context": map[string]string{
				"source":      "ceo-repl",
				"created_by":  "ceo",
				"response":    responseText,
				"actions":     string(actionsJSON),
				"provider_id": providerRecord.ID,
				"model":       responseModel,
				"tokens_used": fmt.Sprintf("%d", tokensUsed),
			},
			"status": models.BeadStatusClosed,
		})
	}

	if responseModel == "" {
		responseModel = providerRecord.SelectedModel
	}
	if responseModel == "" {
		responseModel = providerRecord.Model
	}
	return &ReplResult{
		BeadID:       beadID,
		ProviderID:   providerRecord.ID,
		ProviderName: providerRecord.Name,
		Model:        responseModel,
		Response:     responseText,
		TokensUsed:   tokensUsed,
		LatencyMs:    latencyMs,
	}, nil
}

// extractPersonaFromMessage extracts persona hint from "persona: message" format
// Returns (personaHint, cleanMessage)
