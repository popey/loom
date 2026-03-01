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

func (a *Loom) setupProviderMetrics() {
	if a.metrics == nil || a.providerRegistry == nil {
		return
	}

	a.providerRegistry.SetMetricsCallback(func(providerID string, success bool, latencyMs int64, totalTokens int64, errorCount int64) {
		if a.metrics != nil {
			a.metrics.RecordProviderRequest(providerID, "", success, latencyMs, totalTokens)
		}

		if a.database == nil {
			return
		}
		provider, err := a.database.GetProvider(providerID)
		if err != nil || provider == nil {
			return
		}
		if success {
			provider.RecordSuccess(latencyMs, totalTokens)
		} else {
			provider.RecordFailure()
		}
		_ = a.database.UpsertProvider(provider)

		if a.eventBus != nil {
			_ = a.eventBus.Publish(&eventbus.Event{
				Type: eventbus.EventTypeProviderUpdated,
				Data: map[string]interface{}{
					"provider_id":  providerID,
					"success":      success,
					"latency_ms":   latencyMs,
					"total_tokens": totalTokens,
				},
			})
		}
	})
}
func (a *Loom) GetProviderRegistry() *provider.Registry {
	return a.providerRegistry
}
func (a *Loom) ListProviders() ([]*internalmodels.Provider, error) {
	if a.database == nil {
		return []*internalmodels.Provider{}, nil
	}
	return a.database.ListProviders()
}
func (a *Loom) RegisterProvider(ctx context.Context, p *internalmodels.Provider, apiKeys ...string) (*internalmodels.Provider, error) {
	log.Printf("RegisterProvider called for: %s (type: %s, endpoint: %s)", p.ID, p.Type, p.Endpoint)
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}
	if p.ID == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if p.Type == "" {
		p.Type = "local"
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	// Endpoint is bootstrapped via heartbeats (port/protocol discovery), but keep the existing
	// OpenAI default normalization for compatibility.
	if p.Type != "ollama" {
		p.Endpoint = normalizeProviderEndpoint(p.Endpoint)
	}
	p.LastHeartbeatError = ""
	if p.ConfiguredModel == "" {
		p.ConfiguredModel = p.Model
	}
	if p.ConfiguredModel == "" {
		p.ConfiguredModel = "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-FP8"
	}
	if p.SelectedModel == "" {
		p.SelectedModel = p.ConfiguredModel
	}
	p.Model = p.SelectedModel

	if err := a.database.UpsertProvider(p); err != nil {
		return nil, err
	}

	// Pass API key to the registry so the Protocol gets authentication.
	// Also persist it on the model so it survives restarts.
	regAPIKey := ""
	if len(apiKeys) > 0 {
		regAPIKey = apiKeys[0]
	}
	if regAPIKey != "" && p.APIKey == "" {
		p.APIKey = regAPIKey
		// Re-persist with the key now populated
		_ = a.database.UpsertProvider(p)
	}
	_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
		ID:                     p.ID,
		Name:                   p.Name,
		Type:                   p.Type,
		Endpoint:               p.Endpoint,
		APIKey:                 regAPIKey,
		Model:                  p.SelectedModel,
		ConfiguredModel:        p.ConfiguredModel,
		SelectedModel:          p.SelectedModel,
		Status:                 p.Status,
		LastHeartbeatAt:        p.LastHeartbeatAt,
		LastHeartbeatLatencyMs: p.LastHeartbeatLatencyMs,
	})
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderRegistered,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": p.ID,
				"name":        p.Name,
				"endpoint":    p.Endpoint,
				"model":       p.SelectedModel,
				"configured":  p.ConfiguredModel,
			},
		})
	}

	// Immediately attempt to get models from the provider to validate and update status
	log.Printf("Launching health check goroutine for provider: %s", p.ID)
	go a.checkProviderHealthAndActivate(p.ID)

	return p, nil
}
func (a *Loom) UpdateProvider(ctx context.Context, p *internalmodels.Provider) (*internalmodels.Provider, error) {
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}
	if p == nil {
		return nil, fmt.Errorf("provider cannot be nil")
	}
	if p.ID == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if p.Type == "" {
		p.Type = "local"
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	if p.Type != "ollama" {
		p.Endpoint = normalizeProviderEndpoint(p.Endpoint)
	}
	// If the operator edits a provider, we treat it as needing re-validation.
	p.LastHeartbeatError = ""
	if p.ConfiguredModel == "" {
		p.ConfiguredModel = p.Model
	}
	if p.ConfiguredModel == "" {
		p.ConfiguredModel = "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-FP8"
	}
	if p.SelectedModel == "" {
		p.SelectedModel = p.ConfiguredModel
	}
	p.Model = p.SelectedModel

	if err := a.database.UpsertProvider(p); err != nil {
		return nil, err
	}

	// The DB preserves the existing api_key when the incoming value is empty
	// (see UpsertProvider SQL). Read back the persisted row so the registry
	// and the return value both carry the correct key.
	if p.APIKey == "" {
		if dbProvider, err := a.database.GetProvider(p.ID); err == nil && dbProvider != nil {
			p.APIKey = dbProvider.APIKey
		}
	}

	_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
		ID:                     p.ID,
		Name:                   p.Name,
		Type:                   p.Type,
		Endpoint:               p.Endpoint,
		APIKey:                 p.APIKey,
		Model:                  p.SelectedModel,
		ConfiguredModel:        p.ConfiguredModel,
		SelectedModel:          p.SelectedModel,
		Status:                 p.Status,
		LastHeartbeatAt:        p.LastHeartbeatAt,
		LastHeartbeatLatencyMs: p.LastHeartbeatLatencyMs,
	})
	// Re-probe health whenever a provider is updated so status refreshes
	// immediately rather than waiting for the next restart.
	go a.checkProviderHealthAndActivate(p.ID)

	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderUpdated,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": p.ID,
				"name":        p.Name,
				"endpoint":    p.Endpoint,
				"model":       p.SelectedModel,
				"configured":  p.ConfiguredModel,
			},
		})
	}

	return p, nil
}
func (a *Loom) DeleteProvider(ctx context.Context, providerID string) error {
	if a.database == nil {
		return fmt.Errorf("database not configured")
	}
	_ = a.providerRegistry.Unregister(providerID)
	err := a.database.DeleteProvider(providerID)
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderDeleted,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": providerID,
			},
		})
	}
	return err
}
func (a *Loom) GetProviderModels(ctx context.Context, providerID string) ([]provider.Model, error) {
	return a.providerRegistry.GetModels(ctx, providerID)
}
func (a *Loom) selectBestProviderForRepl() (*internalmodels.Provider, error) {
	providers, err := a.database.ListProviders()
	if err != nil {
		return nil, err
	}

	// With TokenHub as the sole provider, just return the first healthy one.
	for _, p := range providers {
		if p == nil {
			continue
		}
		if p.Status == "healthy" || p.Status == "active" {
			return p, nil
		}
	}

	return nil, fmt.Errorf("no healthy providers available")
}
func normalizeProviderEndpoint(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	// vLLM is typically OpenAI-compatible at /v1.
	if len(endpoint) >= 3 && endpoint[len(endpoint)-3:] == "/v1" {
		return endpoint
	}
	return fmt.Sprintf("%s/v1", strings.TrimSuffix(endpoint, "/"))
}
func (a *Loom) checkProviderHealthAndActivate(providerID string) {
	time.Sleep(300 * time.Millisecond)
	log.Printf("Checking health for provider: %s", providerID)

	// Use a lightweight chat completion as the health probe. This verifies
	// end-to-end connectivity, authentication, and model availability — all
	// better signals than the /v1/models endpoint, which some proxies
	// (e.g. TokenHub) restrict behind a different scope.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	registered, err := a.providerRegistry.Get(providerID)
	if err != nil {
		log.Printf("Provider %s health check failed: %v", providerID, err)
		return
	}
	_, err = registered.Protocol.CreateChatCompletion(ctx, &provider.ChatCompletionRequest{
		Model:     registered.Config.SelectedModel,
		Messages:  []provider.ChatMessage{{Role: "user", Content: "ping"}},
		MaxTokens: 1,
	})
	if err != nil {
		log.Printf("Provider %s health probe failed: %v", providerID, err)
		return
	}

	log.Printf("Provider %s is healthy, activating", providerID)
	if dbProvider, err := a.database.GetProvider(providerID); err == nil && dbProvider != nil {
		dbProvider.Status = "active"
		_ = a.database.UpsertProvider(dbProvider)
		_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
			ID:                     dbProvider.ID,
			Name:                   dbProvider.Name,
			Type:                   dbProvider.Type,
			Endpoint:               dbProvider.Endpoint,
			APIKey:                 dbProvider.APIKey,
			Model:                  dbProvider.SelectedModel,
			ConfiguredModel:        dbProvider.ConfiguredModel,
			SelectedModel:          dbProvider.SelectedModel,
			Status:                 "active",
			LastHeartbeatAt:        dbProvider.LastHeartbeatAt,
			LastHeartbeatLatencyMs: dbProvider.LastHeartbeatLatencyMs,
		})
		log.Printf("Provider %s activated successfully", providerID)
	}

	a.attachProviderToPausedAgents(context.Background(), providerID)
}
