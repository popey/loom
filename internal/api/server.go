package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jordanhubbard/loom/internal/analytics"
	"github.com/jordanhubbard/loom/internal/auth"
	"github.com/jordanhubbard/loom/internal/cache"
	connectorssvc "github.com/jordanhubbard/loom/internal/connectors"
	"github.com/jordanhubbard/loom/internal/files"
	"github.com/jordanhubbard/loom/internal/keymanager"
	"github.com/jordanhubbard/loom/internal/logging"
	"github.com/jordanhubbard/loom/internal/loom"
	"github.com/jordanhubbard/loom/internal/metrics"
	"github.com/jordanhubbard/loom/pkg/config"
	"github.com/jordanhubbard/loom/pkg/models"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ─── Debug instrumentation ─────────────────────────────────────────────────

// debugSeq is an atomic sequence counter for debug log entries.
var debugSeq atomic.Int64

// debugEntry is the canonical JSON structure emitted for every debug event.
// Schema version 1 — see docs/DEBUG.md.
type debugEntry struct {
	TS         string                 `json:"ts"`
	Seq        int64                  `json:"seq"`
	Schema     string                 `json:"schema"`
	DebugLevel string                 `json:"debug_level"`
	Category   string                 `json:"category"`
	Action     string                 `json:"action"`
	Source     string                 `json:"source"`
	Data       map[string]interface{} `json:"data"`
	DurationMS int64                  `json:"duration_ms,omitempty"`
}

// debugPattern describes a URL + method pair that maps to a standard-level semantic event.
type debugPattern struct {
	method   string
	re       *regexp.Regexp
	category string
	action   string
}

// standardDebugPatterns are the API paths emitted at "standard" debug level.
var standardDebugPatterns = []debugPattern{
	// Bead lifecycle
	{"POST", regexp.MustCompile(`^/api/v1/beads$`), "bead_event", "bead created"},
	{"PUT", regexp.MustCompile(`^/api/v1/beads/[^/]+$`), "bead_event", "bead updated"},
	{"PATCH", regexp.MustCompile(`^/api/v1/beads/[^/]+$`), "bead_event", "bead updated"},
	{"DELETE", regexp.MustCompile(`^/api/v1/beads/[^/]+$`), "bead_event", "bead deleted"},
	{"POST", regexp.MustCompile(`^/api/v1/beads/[^/]+/close$`), "bead_event", "bead closed"},
	{"POST", regexp.MustCompile(`^/api/v1/beads/[^/]+/claim$`), "bead_event", "bead claimed"},
	{"POST", regexp.MustCompile(`^/api/v1/beads/[^/]+/block$`), "bead_event", "bead blocked"},
	{"POST", regexp.MustCompile(`^/api/v1/beads/[^/]+/redispatch$`), "bead_event", "bead redispatched"},
	{"POST", regexp.MustCompile(`^/api/v1/beads/[^/]+/annotate$`), "bead_event", "bead annotated"},
	// Agent lifecycle
	{"POST", regexp.MustCompile(`^/api/v1/agents$`), "agent_event", "agent created"},
	{"DELETE", regexp.MustCompile(`^/api/v1/agents/[^/]+$`), "agent_event", "agent deleted"},
	{"POST", regexp.MustCompile(`^/api/v1/agents/[^/]+/start$`), "agent_event", "agent started"},
	{"POST", regexp.MustCompile(`^/api/v1/agents/[^/]+/stop$`), "agent_event", "agent stopped"},
	{"POST", regexp.MustCompile(`^/api/v1/agents/[^/]+/pause$`), "agent_event", "agent paused"},
	{"POST", regexp.MustCompile(`^/api/v1/agents/[^/]+/resume$`), "agent_event", "agent resumed"},
	// Project lifecycle
	{"POST", regexp.MustCompile(`^/api/v1/projects$`), "project_event", "project created"},
	{"POST", regexp.MustCompile(`^/api/v1/projects/bootstrap$`), "project_event", "project bootstrapped"},
	{"DELETE", regexp.MustCompile(`^/api/v1/projects/[^/]+$`), "project_event", "project deleted"},
	{"PUT", regexp.MustCompile(`^/api/v1/projects/[^/]+$`), "project_event", "project updated"},
	{"POST", regexp.MustCompile(`^/api/v1/projects/[^/]+/close$`), "project_event", "project closed"},
}

// streamingPrefixes are path prefixes whose responses are SSE/chunked streams
// and must NOT have their body buffered.
var streamingPrefixes = []string{
	"/api/v1/events",
	"/api/v1/logs/stream",
	"/api/v1/pair",
	"/api/v1/chat/completions",
	"/api/v1/streaming",
}

func isStreamingPath(path string) bool {
	// Catch any path ending in /stream regardless of prefix
	if strings.HasSuffix(path, "/stream") || strings.Contains(path, "/stream?") {
		return true
	}
	for _, p := range streamingPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func matchStandardPattern(method, path string) *debugPattern {
	for i := range standardDebugPatterns {
		p := &standardDebugPatterns[i]
		if p.method == method && p.re.MatchString(path) {
			return p
		}
	}
	return nil
}

// emitDebugLog writes a JSON debug entry to stdout (appears in docker logs).
func (s *Server) emitDebugLog(category, action string, data map[string]interface{}, durationMS int64) {
	lvl := s.config.DebugLevel
	if lvl == "" || lvl == "off" {
		return
	}
	entry := debugEntry{
		TS:         time.Now().UTC().Format(time.RFC3339Nano),
		Seq:        debugSeq.Add(1),
		Schema:     "1",
		DebugLevel: lvl,
		Category:   category,
		Action:     "[LOOM_DEBUG] " + action,
		Source:     "api_middleware",
		Data:       data,
		DurationMS: durationMS,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	fmt.Fprintf(os.Stdout, "%s\n", b)
}

// ──────────────────────────────────────────────────────────────────────────────

// Server represents the HTTP API server
type Server struct {
	app             *loom.Loom
	keyManager      *keymanager.KeyManager
	authManager     *auth.Manager
	analyticsLogger *analytics.Logger
	logManager      *logging.Manager
	cache           *cache.Cache
	config          *config.Config
	fileManager     *files.Manager
	metrics         *metrics.Metrics
	apiFailureMu    sync.Mutex
	apiFailureLast  map[string]time.Time

	// ConnectorService provides location-transparent access to connectors.
	// Uses gRPC remote service when CONNECTORS_SERVICE_ADDR is set,
	// otherwise falls back to the in-process manager.
	connectorService connectorssvc.ConnectorService

	// Circuit breaker for auto-filing API failures as beads.
	// Prevents cascading failures when the bead subsystem itself is broken.
	autoFileCBMu          sync.Mutex
	autoFileConsecFails   int
	autoFileLastFail      time.Time
	autoFileCircuitOpen   bool
	autoFileCircuitOpenAt time.Time
}

// NewServer creates a new API server
func NewServer(arb *loom.Loom, km *keymanager.KeyManager, am *auth.Manager, cfg *config.Config) *Server {
	// Initialize analytics logger with default privacy config
	var analyticsLogger *analytics.Logger
	if arb != nil && arb.GetDatabase() != nil {
		storage, err := analytics.NewDatabaseStorage(arb.GetDatabase().DB())
		if err == nil {
			analyticsLogger = analytics.NewLogger(storage, analytics.DefaultPrivacyConfig())
		}
	}

	// Initialize logging manager
	var logMgr *logging.Manager
	if arb != nil && arb.GetDatabase() != nil {
		logMgr = logging.NewManager(arb.GetDatabase().DB())
	}

	// Initialize cache with config
	var responseCache *cache.Cache
	if cfg != nil && cfg.Cache.Enabled {
		cacheConfig := &cache.Config{
			Enabled:       cfg.Cache.Enabled,
			DefaultTTL:    cfg.Cache.DefaultTTL,
			MaxSize:       cfg.Cache.MaxSize,
			MaxMemoryMB:   cfg.Cache.MaxMemoryMB,
			CleanupPeriod: cfg.Cache.CleanupPeriod,
		}
		// Use defaults if not specified
		if cacheConfig.DefaultTTL == 0 {
			cacheConfig.DefaultTTL = 1 * time.Hour
		}
		if cacheConfig.MaxSize == 0 {
			cacheConfig.MaxSize = 10000
		}
		if cacheConfig.CleanupPeriod == 0 {
			cacheConfig.CleanupPeriod = 5 * time.Minute
		}

		// Use Redis backend if configured, fallback to memory
		if cfg.Cache.Backend == "redis" && cfg.Cache.RedisURL != "" {
			redisCache, err := cache.NewRedisCache(cfg.Cache.RedisURL, cacheConfig)
			if err != nil {
				// Redis failed - fallback to in-memory cache with warning
				fmt.Printf("[WARN] Redis cache initialization failed: %v, falling back to in-memory cache\n", err)
				responseCache = cache.New(cacheConfig)
			} else {
				// Wrap Redis cache to match Cache interface
				responseCache = cache.NewFromRedis(redisCache)
			}
		} else {
			responseCache = cache.New(cacheConfig)
		}
	}

	var fileManager *files.Manager
	if arb != nil {
		fileManager = files.NewManager(arb.GetGitOpsManager())
	}

	// Initialize Prometheus metrics
	promMetrics := metrics.NewMetrics()

	// Initialize connector service: prefer remote gRPC, fall back to local
	var connSvc connectorssvc.ConnectorService
	if addr := os.Getenv("CONNECTORS_SERVICE_ADDR"); addr != "" {
		remote, err := connectorssvc.NewRemoteService(addr)
		if err != nil {
			log.Printf("[API] Could not connect to remote connectors service at %s: %v (falling back to local)", addr, err)
		} else {
			connSvc = remote
			log.Printf("[API] Using remote connectors service at %s", addr)
		}
	}
	if connSvc == nil && arb != nil && arb.GetConnectorManager() != nil {
		connSvc = connectorssvc.NewLocalService(arb.GetConnectorManager())
		log.Printf("[API] Using local connectors service")
	}

	return &Server{
		app:              arb,
		keyManager:       km,
		authManager:      am,
		analyticsLogger:  analyticsLogger,
		logManager:       logMgr,
		cache:            responseCache,
		config:           cfg,
		fileManager:      fileManager,
		connectorService: connSvc,
		metrics:          promMetrics,
		apiFailureLast:   make(map[string]time.Time),
	}
}

// SetupRoutes configures HTTP routes
func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Serve static files
	if s.config.WebUI.Enabled {
		fs := http.FileServer(http.Dir(s.config.WebUI.StaticPath))
		mux.Handle("/static/", http.StripPrefix("/static/", fs))

		// Serve index.html at root
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.ServeFile(w, r, s.config.WebUI.StaticPath+"/index.html")
			} else {
				http.NotFound(w, r)
			}
		})
	}

	// Serve OpenAPI spec
	mux.HandleFunc("/api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./api/openapi.yaml")
	})

	// Documentation browser API
	mux.HandleFunc("/api/v1/docs", s.handleDocs)

	// Health check
	mux.HandleFunc("/api/v1/health", s.handleHealth)

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Auth endpoints
	authHandlers := auth.NewHandlers(s.authManager)
	mux.HandleFunc("/api/v1/auth/login", authHandlers.HandleLogin)
	mux.HandleFunc("/api/v1/auth/refresh", authHandlers.HandleRefreshToken)
	mux.HandleFunc("/api/v1/auth/change-password", authHandlers.HandleChangePassword)
	mux.HandleFunc("/api/v1/auth/api-keys", authHandlers.HandleAPIKeys)
	mux.HandleFunc("/api/v1/auth/api-keys/", authHandlers.HandleAPIKeyByID)
	mux.HandleFunc("/api/v1/auth/me", authHandlers.HandleGetCurrentUser)
	mux.HandleFunc("/api/v1/auth/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandlers.HandleCreateUser(w, r)
		case http.MethodGet:
			authHandlers.HandleListUsers(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Personas
	mux.HandleFunc("/api/v1/personas", s.handlePersonas)
	mux.HandleFunc("/api/v1/personas/", s.handlePersona)

	// Agents
	mux.HandleFunc("/api/v1/agents", s.handleAgents)
	mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	// Projects (includes /projects/{id}/files/*)
	mux.HandleFunc("/api/v1/projects/bootstrap", s.handleBootstrapProject)
	mux.HandleFunc("/api/v1/projects", s.handleProjects)
	mux.HandleFunc("/api/v1/projects/", s.handleProject)

	// Org Charts
	mux.HandleFunc("/api/v1/org-charts/", s.handleOrgChart)

	// Beads
	mux.HandleFunc("/api/v1/beads", s.handleBeads)
	mux.HandleFunc("/api/v1/beads/", s.handleBead)

	// Connectors
	mux.HandleFunc("/api/v1/connectors", s.HandleConnectors)
	mux.HandleFunc("/api/v1/connectors/", s.HandleConnectors)

	// Federation
	mux.HandleFunc("/api/v1/federation/status", s.handleFederationStatus)
	mux.HandleFunc("/api/v1/federation/sync", s.handleFederationSync)

	// Comments (must be registered before other /beads/ routes to avoid conflicts)
	// Note: This is already handled by handleBead which routes to specific sub-paths
	// but we also need a dedicated handler for comments
	// Actually, we'll use a pattern that matches /beads/{id}/comments
	mux.HandleFunc("/api/v1/comments/", s.handleComment)

	// Meetings
	mux.HandleFunc("/api/v1/meetings", s.handleMeetings)
	mux.HandleFunc("/api/v1/meetings/", s.handleMeeting)
	mux.HandleFunc("/api/v1/meetings/active", s.handleActiveMeetings)

	// Conversations
	mux.HandleFunc("/api/v1/conversations", s.handleConversationsList)
	mux.HandleFunc("/api/v1/conversations/", s.handleConversation)

	// Decisions
	mux.HandleFunc("/api/v1/decisions", s.handleDecisions)
	mux.HandleFunc("/api/v1/decisions/", s.handleDecision)

	// File locks
	mux.HandleFunc("/api/v1/file-locks", s.handleFileLocks)
	mux.HandleFunc("/api/v1/file-locks/", s.handleFileLock)

	// Work graph
	mux.HandleFunc("/api/v1/work-graph", s.handleWorkGraph)

	// Providers
	mux.HandleFunc("/api/v1/providers", s.handleProviders)
	mux.HandleFunc("/api/v1/providers/", s.handleProvider)

	// Models
	mux.HandleFunc("/api/v1/models/recommended", s.handleRecommendedModels)

	// System
	mux.HandleFunc("/api/v1/system/status", s.handleSystemStatus)
	mux.HandleFunc("/api/v1/system/state", s.handleSystemState)

	// Work (non-bead prompts)
	mux.HandleFunc("/api/v1/work", s.handleWork)

	// CEO REPL
	mux.HandleFunc("/api/v1/repl", s.handleRepl)

	// Shell command execution
	mux.HandleFunc("/api/v1/commands/execute", s.HandleExecuteCommand)
	mux.HandleFunc("/api/v1/commands", s.HandleGetCommandLogs)
	mux.HandleFunc("/api/v1/commands/", s.HandleGetCommandLogs)

	// Auto-filed bug reports
	mux.HandleFunc("/api/v1/beads/auto-file", s.HandleAutoFileBug)

	// Logging endpoints
	mux.HandleFunc("/api/v1/logs/recent", s.HandleLogsRecent)
	mux.HandleFunc("/api/v1/logs/stream", s.HandleLogsStream)
	mux.HandleFunc("/api/v1/logs/export", s.HandleLogsExport)

	// Chat completions (with streaming support)
	mux.HandleFunc("/api/v1/chat/completions/stream", s.handleStreamChatCompletion)
	mux.HandleFunc("/api/v1/chat/completions", s.handleChatCompletion)

	// Pair-programming chat (SSE streaming with conversation persistence)
	mux.HandleFunc("/api/v1/pair", s.handlePairChat)

	// Git operations
	mux.HandleFunc("/api/v1/projects/git/sync", s.handleGitSync)
	mux.HandleFunc("/api/v1/projects/git/commit", s.handleGitCommit)
	mux.HandleFunc("/api/v1/projects/git/push", s.handleGitPush)
	mux.HandleFunc("/api/v1/projects/git/status", s.handleGitStatus)

	// Analytics and cost tracking
	mux.HandleFunc("/api/v1/analytics/logs", s.handleGetLogs)
	mux.HandleFunc("/api/v1/analytics/stats", s.handleGetLogStats)
	mux.HandleFunc("/api/v1/analytics/export", s.handleExportLogs)
	mux.HandleFunc("/api/v1/analytics/export-stats", s.handleExportStats)
	mux.HandleFunc("/api/v1/analytics/costs", s.handleGetCostReport)
	mux.HandleFunc("/api/v1/analytics/batching", s.handleGetBatchingRecommendations)
	mux.HandleFunc("/api/v1/analytics/change-velocity", s.handleGetChangeVelocity)

	// Debug endpoints
	mux.HandleFunc("/api/v1/debug/capture-ui", s.handleCaptureUI)

	// Cache management
	mux.HandleFunc("/api/v1/cache/stats", s.handleGetCacheStats)
	mux.HandleFunc("/api/v1/cache/config", s.handleGetCacheConfig)
	mux.HandleFunc("/api/v1/cache/clear", s.handleClearCache)
	mux.HandleFunc("/api/v1/cache/invalidate", s.handleInvalidateCache)

	// Cache analysis and optimization
	mux.HandleFunc("/api/v1/cache/analysis", s.handleCacheAnalysis)
	mux.HandleFunc("/api/v1/cache/opportunities", s.handleCacheOpportunities)
	mux.HandleFunc("/api/v1/cache/optimize", s.handleCacheOptimize)
	mux.HandleFunc("/api/v1/cache/recommendations", s.handleCacheRecommendations)

	// Pattern analysis routes
	mux.HandleFunc("/api/v1/patterns/analysis", s.handlePatternAnalysis)
	mux.HandleFunc("/api/v1/patterns/expensive", s.handleExpensivePatterns)
	mux.HandleFunc("/api/v1/patterns/anomalies", s.handleAnomalies)
	mux.HandleFunc("/api/v1/optimizations", s.handleOptimizations)
	mux.HandleFunc("/api/v1/prompts/analysis", s.handlePromptAnalysis)
	mux.HandleFunc("/api/v1/prompts/optimizations", s.handlePromptOptimizations)
	mux.HandleFunc("/api/v1/optimizations/substitutions", s.handleSubstitutions)
	mux.HandleFunc("/api/v1/optimizations/", s.handleOptimizationActions)

	// Health check endpoints
	mux.HandleFunc("/health", s.handleHealthDetail)      // Detailed health
	mux.HandleFunc("/health/live", s.handleHealthLive)   // Liveness probe
	mux.HandleFunc("/health/ready", s.handleHealthReady) // Readiness probe

	// Configuration
	mux.HandleFunc("/api/v1/config/debug", s.handleDebugConfig)
	mux.HandleFunc("/api/v1/config", s.handleConfig)
	mux.HandleFunc("/api/v1/config/export.yaml", s.handleConfigExportYAML)
	mux.HandleFunc("/api/v1/config/import.yaml", s.handleConfigImportYAML)

	// Events (real-time updates and event bus)
	mux.HandleFunc("/api/v1/events/stream", s.handleEventStream)
	mux.HandleFunc("/api/v1/events/stats", s.handleGetEventStats)
	mux.HandleFunc("/api/v1/events", s.handleGetEvents) // GET for history
	// POST /api/v1/events for publishing is available but should be restricted

	// Activity feed
	mux.HandleFunc("/api/v1/activity-feed", s.handleGetActivityFeed)
	mux.HandleFunc("/api/v1/activity-feed/stream", s.handleActivityFeedStream)

	// Notifications
	mux.HandleFunc("/api/v1/notifications", s.handleGetNotifications)
	mux.HandleFunc("/api/v1/notifications/stream", s.handleNotificationStream)
	mux.HandleFunc("/api/v1/notifications/", s.handleNotificationActions)
	mux.HandleFunc("/api/v1/notifications/mark-all-read", s.handleMarkAllRead)
	mux.HandleFunc("/api/v1/notifications/preferences", s.handleNotificationPreferences)

	// Motivations
	mux.HandleFunc("/api/v1/motivations", s.handleMotivations)
	mux.HandleFunc("/api/v1/motivations/", s.handleMotivation)
	mux.HandleFunc("/api/v1/motivations/history", s.handleMotivationHistory)
	mux.HandleFunc("/api/v1/motivations/idle", s.handleIdleState)
	mux.HandleFunc("/api/v1/motivations/roles", s.handleMotivationRoles)
	mux.HandleFunc("/api/v1/motivations/defaults", s.handleMotivationDefaults)

	// Workflows (Phase 4 & 5)
	mux.HandleFunc("/api/v1/workflows", s.handleWorkflows)
	mux.HandleFunc("/api/v1/workflows/start", s.handleWorkflowStart)
	mux.HandleFunc("/api/v1/workflows/", s.handleWorkflow)
	mux.HandleFunc("/api/v1/workflows/executions", s.handleWorkflowExecutions)
	mux.HandleFunc("/api/v1/workflows/analytics", s.handleWorkflowAnalytics)
	mux.HandleFunc("/api/v1/beads/workflow", s.handleBeadWorkflow)

	// Webhooks (external event integration)
	mux.HandleFunc("/api/v1/webhooks/github", s.handleGitHubWebhook)
	mux.HandleFunc("/api/v1/webhooks/openclaw", s.handleOpenClawWebhook)
	mux.HandleFunc("/api/v1/webhooks/status", s.handleWebhookStatus)

	// OpenClaw messaging gateway
	mux.HandleFunc("/api/v1/openclaw/status", s.handleOpenClawStatus)

	// Database export/import
	mux.HandleFunc("/api/v1/export", s.handleExport)
	mux.HandleFunc("/api/v1/import", s.handleImport)

	// Project agent registration (called by containers on startup)
	mux.HandleFunc("/api/v1/project-agents/", s.handleContainerAgents)
	mux.HandleFunc("/api/v1/project-agents/register", s.handleContainerAgents)

	// Apply middleware
	handler := s.loggingMiddleware(mux)
	handler = s.corsMiddleware(handler)
	handler = s.authMiddleware(handler)

	return handler
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "ok",
		"version":        getVersion(),
		"uptime_seconds": int64(time.Since(startTime).Seconds()),
	})
}

// Middleware

// loggingMiddleware logs HTTP requests and emits structured debug events.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lvl := s.config.DebugLevel
		isExtreme := lvl == "extreme"
		isStandard := lvl == "standard" || isExtreme
		streaming := isStreamingPath(r.URL.Path)
		start := time.Now()

		// For extreme: buffer the request body so we can log it (non-streaming only)
		var reqBodyPreview string
		if isExtreme && !streaming && r.Body != nil {
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, io.LimitReader(r.Body, 64*1024))
			reqBodyPreview = buf.String()
			r.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
		}

		// Emit incoming request event (extreme only)
		if isExtreme {
			reqData := map[string]interface{}{
				"method":         r.Method,
				"path":           r.URL.Path,
				"query":          r.URL.RawQuery,
				"remote_addr":    r.RemoteAddr,
				"user_agent":     r.Header.Get("User-Agent"),
				"content_type":   r.Header.Get("Content-Type"),
				"content_length": r.ContentLength,
			}
			if reqBodyPreview != "" {
				if len(reqBodyPreview) > 1024 {
					reqData["body_preview"] = reqBodyPreview[:1024] + "…"
				} else {
					reqData["body_preview"] = reqBodyPreview
				}
			}
			s.emitDebugLog("api_request", r.Method+" "+r.URL.Path, reqData, 0)
		}

		// Wrap response writer to capture status code and optional response body
		recorder := &statusRecorder{
			ResponseWriter: w,
			captureBody:    isExtreme && !streaming,
		}
		next.ServeHTTP(recorder, r)

		statusCode := recorder.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		durationMS := time.Since(start).Milliseconds()

		// Auto-file circuit breaker (existing behaviour)
		s.recordAPIFailure(r, statusCode)

		// Emit response event
		if isExtreme || (isStandard && statusCode >= 400) {
			stdMatch := matchStandardPattern(r.Method, r.URL.Path)
			category := "api_response"
			action := fmt.Sprintf("%s %s → %d", r.Method, r.URL.Path, statusCode)
			if statusCode >= 400 {
				category = "api_error"
			} else if stdMatch != nil {
				category = stdMatch.category
				action = stdMatch.action + fmt.Sprintf(" (%d)", statusCode)
			}

			respData := map[string]interface{}{
				"method": r.Method,
				"path":   r.URL.Path,
				"status": statusCode,
			}
			if r.URL.RawQuery != "" {
				respData["query"] = r.URL.RawQuery
			}
			if isExtreme {
				body := recorder.body.String()
				if len(body) > 1024 {
					body = body[:1024] + "…"
				}
				respData["body_preview"] = body
				respData["body_bytes"] = recorder.body.Len()
			}
			minLevel := "extreme"
			if stdMatch != nil || statusCode >= 400 {
				minLevel = "standard"
			}
			_ = minLevel // used conceptually; emitDebugLog respects the global level
			s.emitDebugLog(category, action, respData, durationMS)
		}
	})
}

// handleDebugConfig returns the server-configured debug level.
// Public endpoint — no auth required.
func (s *Server) handleDebugConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	lvl := s.config.DebugLevel
	if lvl == "" {
		lvl = "off"
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"level": lvl})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode  int
	captureBody bool
	body        bytes.Buffer
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher to support streaming responses (SSE, etc.)
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

const debugBodyCap = 64 * 1024 // 64 KB max capture per response

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	if r.captureBody && r.body.Len() < debugBodyCap {
		remaining := debugBodyCap - r.body.Len()
		if len(b) > remaining {
			r.body.Write(b[:remaining])
		} else {
			r.body.Write(b)
		}
	}
	return r.ResponseWriter.Write(b)
}

func (s *Server) recordAPIFailure(r *http.Request, statusCode int) {
	if statusCode < http.StatusInternalServerError {
		return
	}
	if r == nil || r.URL == nil || s.app == nil {
		return
	}

	// Don't auto-file failures on the auto-file endpoint itself (prevents loops)
	if strings.HasSuffix(r.URL.Path, "/auto-file") {
		return
	}

	// Check circuit breaker before attempting to file
	if s.isAutoFileCircuitOpen() {
		return
	}

	key := fmt.Sprintf("%s %s %d", r.Method, r.URL.Path, statusCode)
	if s.shouldThrottleFailure(key, 2*time.Minute) {
		return
	}

	// Check bead subsystem health before filing
	if !s.isBeadSubsystemHealthy() {
		return
	}

	projectID := s.defaultProjectID()
	if projectID == "" {
		return
	}

	title := fmt.Sprintf("P0 - API failure %s", key)
	description := fmt.Sprintf(
		"API request failed\n\nMethod: %s\nPath: %s\nStatus: %d\nQuery: %s\nRemote: %s\nUser: %s\nTimestamp: %s\n",
		r.Method,
		r.URL.Path,
		statusCode,
		r.URL.RawQuery,
		r.RemoteAddr,
		auth.GetUserIDFromRequest(r),
		time.Now().UTC().Format(time.RFC3339),
	)

	_, err := s.app.CreateBead(title, description, models.BeadPriority(0), "task", projectID)
	s.recordAutoFileResult(err)
}

const (
	autoFileCBMaxFails   = 3               // Trip circuit after 3 consecutive failures
	autoFileCBResetAfter = 5 * time.Minute // Re-attempt after 5 minutes
)

func (s *Server) isAutoFileCircuitOpen() bool {
	s.autoFileCBMu.Lock()
	defer s.autoFileCBMu.Unlock()

	if !s.autoFileCircuitOpen {
		return false
	}
	// Half-open: allow retry after cooldown
	if time.Since(s.autoFileCircuitOpenAt) > autoFileCBResetAfter {
		s.autoFileCircuitOpen = false
		s.autoFileConsecFails = 0
		return false
	}
	return true
}

func (s *Server) recordAutoFileResult(err error) {
	s.autoFileCBMu.Lock()
	defer s.autoFileCBMu.Unlock()

	if err != nil {
		s.autoFileConsecFails++
		s.autoFileLastFail = time.Now()
		if s.autoFileConsecFails >= autoFileCBMaxFails {
			s.autoFileCircuitOpen = true
			s.autoFileCircuitOpenAt = time.Now()
			fmt.Printf("[WARN] Auto-file circuit breaker tripped after %d consecutive failures\n", s.autoFileConsecFails)
		}
	} else {
		s.autoFileConsecFails = 0
		s.autoFileCircuitOpen = false
	}
}

func (s *Server) isBeadSubsystemHealthy() bool {
	if s.app == nil {
		return false
	}
	bm := s.app.GetBeadsManager()
	if bm == nil {
		return false
	}
	// Quick check: can we list beads without error?
	_, err := bm.ListBeads(nil)
	return err == nil
}

func (s *Server) shouldThrottleFailure(key string, window time.Duration) bool {
	if key == "" {
		return true
	}
	now := time.Now()
	s.apiFailureMu.Lock()
	defer s.apiFailureMu.Unlock()
	if last, ok := s.apiFailureLast[key]; ok && now.Sub(last) < window {
		return true
	}
	s.apiFailureLast[key] = now
	return false
}

func (s *Server) defaultProjectID() string {
	if s.config != nil && s.config.GetSelfProjectID() != "" {
		return s.config.GetSelfProjectID()
	}
	if s.app == nil {
		return ""
	}
	if pm := s.app.GetProjectManager(); pm != nil {
		if projects := pm.ListProjects(); len(projects) > 0 && projects[0] != nil {
			return projects[0].ID
		}
	}
	return ""
}

// corsMiddleware handles CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		if len(s.config.Security.AllowedOrigins) > 0 {
			origin := r.Header.Get("Origin")
			for _, allowedOrigin := range s.config.Security.AllowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
					break
				}
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")

		// Handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware handles authentication
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check endpoints (all variants for monitoring/probes)
		if r.URL.Path == "/api/v1/health" ||
			r.URL.Path == "/health" ||
			r.URL.Path == "/health/live" ||
			r.URL.Path == "/health/ready" ||
			r.URL.Path == "/api/v1/auth/login" ||
			r.URL.Path == "/api/v1/auth/refresh" ||
			r.URL.Path == "/api/v1/config/debug" ||
			r.URL.Path == "/" ||
			r.URL.Path == "/api/openapi.yaml" ||
			r.URL.Path == "/api/v1/events/stream" ||
			r.URL.Path == "/api/v1/chat/completions/stream" ||
			r.URL.Path == "/api/v1/chat/completions" ||
			r.URL.Path == "/api/v1/pair" ||
			r.URL.Path == "/api/v1/webhooks/openclaw" ||
			strings.HasPrefix(r.URL.Path, "/api/v1/project-agents/") ||
			strings.HasPrefix(r.URL.Path, "/static/") ||
			strings.HasPrefix(r.URL.Path, "/api/v1/motivations/") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth if disabled — treat all requests as admin
		if !s.config.Security.EnableAuth || s.authManager == nil {
			r.Header.Set("X-User-ID", "admin")
			r.Header.Set("X-Username", "admin")
			r.Header.Set("X-Role", "admin")
			next.ServeHTTP(w, r)
			return
		}

		// Apply JWT/API key auth
		s.authManager.Middleware("")(next).ServeHTTP(w, r)
	})
}

// Helper functions

// getUserFromContext extracts the user from request headers (set by auth middleware)
func (s *Server) getUserFromContext(r *http.Request) *auth.User {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		return nil
	}

	username := r.Header.Get("X-Username")
	role := r.Header.Get("X-Role")

	return &auth.User{
		ID:       userID,
		Username: username,
		Role:     role,
		IsActive: true,
	}
}

// respondJSON writes a JSON response
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	body, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
	w.Write([]byte("\n"))
}

// respondError writes an error response
func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]string{"error": message})
}

// parseJSON parses JSON request body
func (s *Server) parseJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// extractID extracts ID from URL path
func (s *Server) extractID(path, prefix string) string {
	// Remove prefix and any trailing slash
	id := strings.TrimPrefix(path, prefix)
	id = strings.TrimPrefix(id, "/")
	id = strings.TrimSuffix(id, "/")

	// Handle sub-paths (e.g., /api/v1/beads/123/claim)
	parts := strings.Split(id, "/")
	if len(parts) > 0 {
		return parts[0]
	}

	return id
}

// GetMetrics returns the Prometheus metrics instance
func (s *Server) GetMetrics() *metrics.Metrics {
	return s.metrics
}
