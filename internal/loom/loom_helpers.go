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

func isSSHRepo(repo string) bool {
	repo = strings.TrimSpace(repo)
	return strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://")
}
func roleFromPersonaName(personaName string) string {
	personaName = strings.TrimSpace(personaName)
	if strings.HasPrefix(personaName, "default/") {
		return strings.TrimPrefix(personaName, "default/")
	}
	if strings.HasPrefix(personaName, "projects/") {
		parts := strings.Split(personaName, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	if strings.Contains(personaName, "/") {
		parts := strings.Split(personaName, "/")
		return parts[len(parts)-1]
	}
	return personaName
}

func capitalizeAcronyms(name string) string {
	// Only replace whole words (space-bounded or at start/end)
	words := strings.Split(name, " ")
	acronyms := map[string]string{
		"Ceo": "CEO",
		"Cfo": "CFO",
		"Qa":  "QA",
	}
	for i, word := range words {
		if replacement, ok := acronyms[word]; ok {
			words[i] = replacement
		}
	}
	return strings.Join(words, " ")
}
func normalizeGitAuthMethod(repo string, method models.GitAuthMethod) models.GitAuthMethod {
	if method != "" {
		return method
	}
	if repo == "" || repo == "." {
		return models.GitAuthNone
	}
	return models.GitAuthSSH
}
func normalizeGitStrategy(strategy models.GitStrategy) models.GitStrategy {
	if strategy != "" {
		return strategy
	}
	return models.GitStrategyDirect
}
func (a *Loom) allowedRoleSet() map[string]struct{} {
	roles := a.config.Agents.AllowedRoles
	if len(roles) == 0 {
		roles = rolesForProfile(a.config.Agents.CorpProfile)
	}
	if len(roles) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(strings.ToLower(role))
		if role == "" {
			continue
		}
		set[role] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}
func rolesForProfile(profile string) []string {
	profile = strings.TrimSpace(strings.ToLower(profile))
	switch profile {
	case "startup":
		return []string{"ceo", "engineering-manager", "web-designer"}
	case "solo":
		return []string{"ceo", "engineering-manager"}
	case "full", "enterprise", "":
		return nil
	default:
		return nil
	}
}

	role = strings.TrimSpace(strings.ToLower(role))
	if strings.Contains(role, "/") {
		parts := strings.Split(role, "/")
		role = parts[len(parts)-1]
	}
	if idx := strings.Index(role, "("); idx != -1 {
		role = strings.TrimSpace(role[:idx])
	}
	role = strings.ReplaceAll(role, "_", "-")
	role = strings.ReplaceAll(role, " ", "-")
	return role
}

	risk, reasons := assessFixRisk(bead.Description)
	log.Printf("[AutoApproval] Bead %s risk=%s reasons=%v", bead.ID, risk, reasons)

	if risk != "low" {
		log.Printf("[AutoApproval] Bead %s requires manual CEO review (risk=%s)", bead.ID, risk)
		return
	}

	// Wait briefly so the bead is fully persisted before we close it
	time.Sleep(2 * time.Second)

	reason := fmt.Sprintf("Auto-approved (risk=%s): %s", risk, strings.Join(reasons, "; "))
	if err := a.CloseBead(bead.ID, reason); err != nil {
		log.Printf("[AutoApproval] Failed to auto-approve bead %s: %v", bead.ID, err)
		return
	}
	log.Printf("[AutoApproval] Auto-approved code fix proposal %s", bead.ID)
}

// assessFixRisk evaluates the risk level of a code fix proposal.
	lower := strings.ToLower(description)
	var reasons []string

	// High-risk indicators
	highRiskPatterns := []string{
		"security", "authentication", "authorization", "password",
		"encryption", "token", "secret", "credential",
		"database migration", "schema change", "drop table",
		"delete all", "rm -rf", "force push",
	}
	for _, p := range highRiskPatterns {
		if strings.Contains(lower, p) {
			return "high", []string{"contains security/destructive keyword: " + p}
		}
	}
	// Look for patterns like "**Original Bug:** ac-001" or "Original Bug: bd-123"
	patterns := []string{
		"**Original Bug:** ",
		"Original Bug: ",
		"**Original Bug:**",
	}

	for _, pattern := range patterns {
		idx := strings.Index(description, pattern)
		if idx >= 0 {
			// Extract the bead ID after the pattern
			start := idx + len(pattern)
			end := start
			for end < len(description) && ((description[end] >= 'a' && description[end] <= 'z') ||
				(description[end] >= '0' && description[end] <= '9') ||
				description[end] == '-') {
				end++
			}
			if end > start {
				bugID := strings.TrimSpace(description[start:end])
				return bugID
			}
		}
	}

	return ""
	blocked := a.decisionManager.GetBlockedBeads(decisionID)

	for _, beadID := range blocked {
		if err := a.beadsManager.UnblockBead(beadID, decisionID); err != nil {
			return fmt.Errorf("failed to unblock bead %s: %w", beadID, err)
		}
	}

	return nil
}

	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[DispatchLoop] debug write to %s failed: %v", path, err)
	}
}

