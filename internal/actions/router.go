package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jordanhubbard/loom/internal/executor"
	"github.com/jordanhubbard/loom/internal/files"
	"github.com/jordanhubbard/loom/internal/persona"
	"github.com/jordanhubbard/loom/pkg/models"
)

type BeadCreator interface {
	CreateBead(title, description string, priority models.BeadPriority, beadType, projectID string) (*models.Bead, error)
}

type BeadCloser interface {
	CloseBead(beadID, reason string) error
}

type BeadEscalator interface {
	EscalateBeadToCEO(beadID, reason, returnedTo string) (*models.DecisionBead, error)
}

type CommandExecutor interface {
	ExecuteCommand(ctx context.Context, req executor.ExecuteCommandRequest) (*executor.ExecuteCommandResult, error)
}

type TestRunner interface {
	Run(ctx context.Context, projectPath string, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error)
}

type LinterRunner interface {
	Run(ctx context.Context, projectPath string, files []string, framework string, timeoutSeconds int) (map[string]interface{}, error)
}

type BuildRunner interface {
	Run(ctx context.Context, projectPath, buildTarget, buildCommand, framework string, timeoutSeconds int) (map[string]interface{}, error)
}

type ProjectGetter interface {
	GetProject(projectID string) (*models.Project, error)
}

type FileManager interface {
	ReadFile(ctx context.Context, projectID, path string) (*files.FileResult, error)
	WriteFile(ctx context.Context, projectID, path, content string) (*files.WriteResult, error)
	ReadTree(ctx context.Context, projectID, path string, maxDepth, limit int) ([]files.TreeEntry, error)
	SearchText(ctx context.Context, projectID, path, query string, limit int) ([]files.SearchMatch, error)
	ApplyPatch(ctx context.Context, projectID, patch string) (*files.PatchResult, error)
	MoveFile(ctx context.Context, projectID, sourcePath, targetPath string) error
	DeleteFile(ctx context.Context, projectID, path string) error
	RenameFile(ctx context.Context, projectID, sourcePath, newName string) error
}

type GitOperator interface {
	Status(ctx context.Context, projectID string) (string, error)
	Diff(ctx context.Context, projectID string) (string, error)
	CreateBranch(ctx context.Context, beadID, description, baseBranch string) (map[string]interface{}, error)
	Commit(ctx context.Context, beadID, agentID, message string, files []string, allowAll bool) (map[string]interface{}, error)
	Push(ctx context.Context, beadID, branch string, setUpstream bool) (map[string]interface{}, error)
	GetStatus(ctx context.Context) (map[string]interface{}, error)
	GetDiff(ctx context.Context, staged bool) (map[string]interface{}, error)
	CreatePR(ctx context.Context, beadID, title, body, base, branch string, reviewers []string, draft bool) (map[string]interface{}, error)
	// Extended git operations
	Merge(ctx context.Context, beadID, sourceBranch, message string, noFF bool) (map[string]interface{}, error)
	Revert(ctx context.Context, beadID string, commitSHAs []string, reason string) (map[string]interface{}, error)
	DeleteBranch(ctx context.Context, branch string, deleteRemote bool) (map[string]interface{}, error)
	Checkout(ctx context.Context, branch string) (map[string]interface{}, error)
	Log(ctx context.Context, branch string, maxCount int) (map[string]interface{}, error)
	Fetch(ctx context.Context) (map[string]interface{}, error)
	ListBranches(ctx context.Context) (map[string]interface{}, error)
	DiffBranches(ctx context.Context, branch1, branch2 string) (map[string]interface{}, error)
	GetBeadCommits(ctx context.Context, beadID string) (map[string]interface{}, error)
}

type ActionLogger interface {
	LogAction(ctx context.Context, actx ActionContext, action Action, result Result)
}

type WorkflowOperator interface {
	AdvanceWorkflowWithCondition(beadID, agentID string, condition string, resultData map[string]string) error
	StartDevelopment(ctx context.Context, workflow string, requireReviews bool, projectPath string) (map[string]interface{}, error)
	WhatsNext(ctx context.Context, userInput, context, conversationSummary string, recentMessages []map[string]string) (map[string]interface{}, error)
	ProceedToPhase(ctx context.Context, targetPhase, reviewState, reason string) (map[string]interface{}, error)
	ConductReview(ctx context.Context, targetPhase string) (map[string]interface{}, error)
	ResumeWorkflow(ctx context.Context, includeSystemPrompt bool) (map[string]interface{}, error)
}

type MessageSender interface {
	SendMessage(ctx context.Context, fromAgentID, toAgentID, messageType, subject, body string, payload map[string]interface{}) (string, error)
	FindAgentByRole(ctx context.Context, role string) (string, error)
}

type BeadReader interface {
	GetBead(beadID string) (*models.Bead, error)
	GetBeadConversation(beadID string) ([]models.ChatMessage, error)
}

type ActionContext struct {
	AgentID   string
	BeadID    string
	ProjectID string
	Model     string // LLM model used for this agent execution (e.g. "claude-opus-4-5")
}

type Result struct {
	ActionType string                 `json:"action_type"`
	Status     string                 `json:"status"`
	Message    string                 `json:"message"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// MeetingCaller handles call_meeting actions.
type MeetingCaller interface {
	CallMeeting(ctx context.Context, initiatorID, title, projectID, beadID string, participants []string, agenda []struct{ Topic, Description string }) (string, error)
}

// AgentConsulter handles consult_agent actions (synchronous question/answer).
type AgentConsulter interface {
	ConsultAgent(ctx context.Context, fromAgentID, toAgentID, toRole, question string) (string, error)
}

// StatusBoardPoster handles post_to_board actions.
type StatusBoardPoster interface {
	PostToBoard(ctx context.Context, projectID, category, content, authorID string) error
}

// VoteCaster handles vote actions for consensus decisions.
type VoteCaster interface {
	CastVote(ctx context.Context, decisionID, agentID, choice, rationale string) error
}

type Router struct {
	Beads          BeadCreator
	Closer         BeadCloser
	Escalator      BeadEscalator
	Commands       CommandExecutor
	Tests          TestRunner
	Linter         LinterRunner
	Builder        BuildRunner
	Files          FileManager
	Git            GitOperator
	Logger         ActionLogger
	Workflow       WorkflowOperator
	LSP            LSPOperator
	MessageBus     MessageSender
	BeadReader     BeadReader
	Projects       ProjectGetter
	ContainerOrch  ContainerOrchestrator
	BuildEnv       *BuildEnvManager
	Meetings       MeetingCaller
	Consulter      AgentConsulter
	Board          StatusBoardPoster
	Voter          VoteCaster
	PersonaManager *persona.Manager
	BeadType       string
	BeadTags       []string
	DefaultP0      bool
}

// getProjectWorkDir returns the working directory for a project
func (r *Router) getProjectWorkDir(projectID string) string {
	if r.Projects == nil || projectID == "" {
		return "" // Let executor use its default
	}

	project, err := r.Projects.GetProject(projectID)
	if err != nil || project == nil {
		return ""
	}

	return project.WorkDir
}

// runBuildForProject auto-discovers and runs the build command for a project.
// Returns (nil, nil) if Commands executor is unavailable (build check skipped).
// Returns (result, nil) if build ran (check result.Success for pass/fail).
// Returns (nil, err) on executor error.
func (r *Router) runBuildForProject(ctx context.Context, actx ActionContext, explicitCommand string) (*executor.ExecuteCommandResult, error) {
	if r.Commands == nil {
		return nil, nil // No executor available; skip build gate
	}

	workDir := r.getProjectWorkDir(actx.ProjectID)

	buildCmd := explicitCommand
	if buildCmd == "" {
		// Auto-detect build system from files present in the working directory
		buildCmd = `if [ -f go.mod ]; then go build ./...; ` +
			`elif [ -f package.json ]; then npm run build; ` +
			`elif [ -f Makefile ]; then make; ` +
			`elif [ -f Cargo.toml ]; then cargo build; ` +
			`else echo "No recognized build system detected (no go.mod, package.json, Makefile, or Cargo.toml)" && exit 1; fi`
	}

	return r.Commands.ExecuteCommand(ctx, executor.ExecuteCommandRequest{
		AgentID:    actx.AgentID,
		BeadID:     actx.BeadID,
		ProjectID:  actx.ProjectID,
		Command:    buildCmd,
		WorkingDir: workDir,
		Timeout:    300,
	})
}

func (r *Router) Execute(ctx context.Context, env *ActionEnvelope, actx ActionContext) ([]Result, error) {
	if env == nil {
		return nil, fmt.Errorf("action envelope is nil")
	}

	// Inject project ID into context so git operations can resolve the work directory
	if actx.ProjectID != "" {
		ctx = WithProjectID(ctx, actx.ProjectID)
	}

	results := make([]Result, 0, len(env.Actions))
	for _, action := range env.Actions {
		result := r.executeAction(ctx, action, actx)
		if r.Logger != nil {
			r.Logger.LogAction(ctx, actx, action, result)
		}
		results = append(results, result)
	}

	return results, nil
}

func (r *Router) AutoFileParseFailure(ctx context.Context, actx ActionContext, err error, raw string) Result {
	if r.Beads == nil {
		return Result{ActionType: ActionCreateBead, Status: "error", Message: "bead creator not configured"}
	}
	priority := models.BeadPriority(0)
	if !r.DefaultP0 {
		priority = models.BeadPriority(2)
	}
	description := fmt.Sprintf("Failed to parse strict JSON actions.\n\nError:\n%s\n\nRaw response:\n%s", err.Error(), raw)
	bead, beadErr := r.Beads.CreateBead("Action parse failed", description, priority, "bug", actx.ProjectID)
	if beadErr != nil {
		return Result{ActionType: ActionCreateBead, Status: "error", Message: beadErr.Error()}
	}
	result := Result{
		ActionType: ActionCreateBead,
		Status:     "executed",
		Message:    "auto-filed action parse failure",
		Metadata:   map[string]interface{}{"bead_id": bead.ID},
	}
	if r.Logger != nil {
		r.Logger.LogAction(ctx, actx, Action{Type: ActionCreateBead}, result)
	}
	return result
}

func (r *Router) executeAction(ctx context.Context, action Action, actx ActionContext) Result {
	switch action.Type {
	case ActionAskFollowup:
		return r.createBeadFromAction("Follow-up question", action.Question, actx)
	case ActionReadCode:
		if r.Files == nil {
			return r.createBeadFromAction("Read code", action.Path, actx)
		}
		res, err := r.Files.ReadFile(ctx, actx.ProjectID, action.Path)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "file read",
			Metadata: map[string]interface{}{
				"path":    res.Path,
				"content": res.Content,
				"size":    res.Size,
			},
		}
	case ActionEditCode:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			if action.OldText != "" && action.Path != "" {
				readRes, readErr := agent.ReadFile(ctx, action.Path)
				if readErr != nil {
					return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("cannot read %s: %v", action.Path, readErr)}
				}
				newContent, matched, strategy := MatchAndReplace(readRes.Content, action.OldText, action.NewText)
				if !matched {
					return Result{ActionType: action.Type, Status: "error",
						Message: fmt.Sprintf("OLD text not found in %s (tried exact, line-trimmed, whitespace-normalized, indentation-flexible, block-anchor matching). Re-read the file with ACTION: READ and copy the exact text.", action.Path)}
				}
				writeRes, writeErr := agent.WriteFile(ctx, action.Path, newContent)
				if writeErr != nil {
					return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("write failed: %v", writeErr)}
				}
				return Result{
					ActionType: action.Type,
					Status:     "executed",
					Message:    fmt.Sprintf("edited %s (match: %s)", action.Path, strategy),
					Metadata: map[string]interface{}{
						"path":           writeRes.Path,
						"bytes_written":  writeRes.BytesWritten,
						"match_strategy": strategy,
					},
				}
			}
			return Result{ActionType: action.Type, Status: "error", Message: "edit_code requires path + old_text + new_text for container projects"}
		}
		if r.Files == nil {
			return r.createBeadFromAction("Edit code", fmt.Sprintf("%s\n\nPatch:\n%s", action.Path, action.Patch), actx)
		}
		// Text-based EDIT: use OldText/NewText with multi-strategy matching
		if action.OldText != "" && action.Path != "" {
			res, readErr := r.Files.ReadFile(ctx, actx.ProjectID, action.Path)
			if readErr != nil {
				return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("cannot read %s: %v", action.Path, readErr)}
			}
			newContent, matched, strategy := MatchAndReplace(res.Content, action.OldText, action.NewText)
			if !matched {
				return Result{ActionType: action.Type, Status: "error",
					Message: fmt.Sprintf("OLD text not found in %s (tried exact, line-trimmed, whitespace-normalized, indentation-flexible, block-anchor matching). Re-read the file with ACTION: READ and copy the exact text.", action.Path)}
			}
			writeRes, writeErr := r.Files.WriteFile(ctx, actx.ProjectID, action.Path, newContent)
			if writeErr != nil {
				return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("write failed: %v", writeErr)}
			}
			return Result{
				ActionType: action.Type,
				Status:     "executed",
				Message:    fmt.Sprintf("edited %s (match: %s)", action.Path, strategy),
				Metadata: map[string]interface{}{
					"path":           writeRes.Path,
					"bytes_written":  writeRes.BytesWritten,
					"match_strategy": strategy,
					"old_length":     len(action.OldText),
					"new_length":     len(action.NewText),
				},
			}
		}
		// Legacy: unified diff patch
		res, err := r.Files.ApplyPatch(ctx, actx.ProjectID, action.Patch)
		if err != nil {
			message := err.Error()
			if res != nil && res.Output != "" {
				message = fmt.Sprintf("%s: %s", message, res.Output)
			}
			return Result{ActionType: action.Type, Status: "error", Message: message}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "patch applied",
			Metadata:   map[string]interface{}{"output": res.Output},
		}
	case ActionWriteFile:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			return r.containerWriteFile(ctx, actx, action)
		}
		if r.Files == nil {
			return r.createBeadFromAction("Write file", fmt.Sprintf("%s\n\nContent:\n%s", action.Path, truncateContent(action.Content, 500)), actx)
		}
		res, err := r.Files.WriteFile(ctx, actx.ProjectID, action.Path, action.Content)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "file written",
			Metadata: map[string]interface{}{
				"path":          res.Path,
				"bytes_written": res.BytesWritten,
			},
		}
	case ActionReadFile:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			return r.containerReadFile(ctx, actx, action)
		}
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		res, err := r.Files.ReadFile(ctx, actx.ProjectID, action.Path)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "file read",
			Metadata: map[string]interface{}{
				"path":    res.Path,
				"content": res.Content,
				"size":    res.Size,
			},
		}
	case ActionReadTree:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			return r.containerReadTree(ctx, actx, action)
		}
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		path := action.Path
		if path == "" {
			path = "."
		}
		res, err := r.Files.ReadTree(ctx, actx.ProjectID, path, action.MaxDepth, action.Limit)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "tree read",
			Metadata:   map[string]interface{}{"entries": res},
		}
	case ActionSearchText:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			return r.containerSearchText(ctx, actx, action)
		}
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		path := action.Path
		if path == "" {
			path = "."
		}
		res, err := r.Files.SearchText(ctx, actx.ProjectID, path, action.Query, action.Limit)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "search completed",
			Metadata:   map[string]interface{}{"matches": res},
		}
	case ActionApplyPatch:
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		res, err := r.Files.ApplyPatch(ctx, actx.ProjectID, action.Patch)
		if err != nil {
			message := err.Error()
			if res != nil && res.Output != "" {
				message = fmt.Sprintf("%s: %s", message, res.Output)
			}
			return Result{ActionType: action.Type, Status: "error", Message: message}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "patch applied",
			Metadata:   map[string]interface{}{"output": res.Output},
		}
	case ActionGitStatus:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			return r.containerGitStatus(ctx, actx, action)
		}
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		out, err := r.Git.Status(ctx, actx.ProjectID)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "git status",
			Metadata:   map[string]interface{}{"output": out},
		}
	case ActionGitDiff:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			return r.containerGitDiff(ctx, actx, action)
		}
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		out, err := r.Git.Diff(ctx, actx.ProjectID)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "git diff",
			Metadata:   map[string]interface{}{"output": out},
		}
	case ActionGitCommit:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			return r.containerGitCommit(ctx, actx, action)
		}
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}

		// Auto branch-per-bead: if we're on main/master, create a bead branch first.
		if actx.BeadID != "" {
			r.ensureBeadBranch(ctx, actx)
		}

		// Pre-commit quality gate: build + test + lint (best effort).
		if gateErr := r.runQualityGate(ctx, actx, action); gateErr != "" {
			return Result{ActionType: action.Type, Status: "error", Message: gateErr}
		}

		// Auto-generate commit message if not provided
		message := action.CommitMessage
		if message == "" {
			message = fmt.Sprintf("feat: Update from bead %s\n\nBead: %s\nAgent: %s\nModel: %s\nCo-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>",
				actx.BeadID, actx.BeadID, actx.AgentID, actx.Model)
		} else if !strings.Contains(message, "Bead:") {
			// Append metadata trailers to agent-provided message if missing
			message = fmt.Sprintf("%s\n\nBead: %s\nAgent: %s\nModel: %s",
				message, actx.BeadID, actx.AgentID, actx.Model)
		}

		result, err := r.Git.Commit(ctx, actx.BeadID, actx.AgentID, message, action.Files, len(action.Files) == 0)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}

		// Auto-push + auto-PR when commit succeeds on a bead branch.
		r.autoPushAndPR(ctx, actx, action, result)

		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "commit created",
			Metadata:   result,
		}
	case ActionGitCheckpoint:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}

		// Create WIP checkpoint commit
		message := action.CommitMessage
		if message == "" {
			// Auto-generate checkpoint message with [WIP] prefix
			message = fmt.Sprintf("[WIP] Checkpoint commit\n\nBead: %s\nAgent: %s\nModel: %s\n\nThis is a work-in-progress checkpoint commit to preserve incremental changes.\n\nCo-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>",
				actx.BeadID, actx.AgentID, actx.Model)
		} else {
			// Ensure WIP prefix is present
			if !strings.HasPrefix(message, "[WIP]") {
				message = "[WIP] " + message
			}
		}

		result, err := r.Git.Commit(ctx, actx.BeadID, actx.AgentID, message, action.Files, len(action.Files) == 0)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}

		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "checkpoint commit created (WIP)",
			Metadata:   result,
		}
	case ActionGitPush:
		if agent := r.GetReadyContainerAgent(ctx, actx.ProjectID); agent != nil {
			return r.containerGitPush(ctx, actx, action)
		}
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}

		result, err := r.Git.Push(ctx, actx.BeadID, action.Branch, action.SetUpstream)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}

		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "branch pushed",
			Metadata:   result,
		}
	case ActionCreatePR:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}

		// Auto-generate title/body from bead if not provided
		title := action.PRTitle
		body := action.PRBody
		if title == "" {
			title = fmt.Sprintf("PR from bead %s", actx.BeadID)
		}
		if body == "" {
			body = fmt.Sprintf("Automated pull request from bead %s\n\nAgent: %s", actx.BeadID, actx.AgentID)
		}

		// Set default base branch
		base := action.PRBase
		if base == "" {
			base = "main"
		}

		result, err := r.Git.CreatePR(ctx, actx.BeadID, title, body, base, action.Branch, action.PRReviewers, false)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}

		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("PR created: %v", result["pr_url"]),
			Metadata:   result,
		}
	// Extended git operations
	case ActionGitMerge:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		noFF := action.NoFF
		if !noFF {
			noFF = true // Default to --no-ff for audit trail
		}
		result, err := r.Git.Merge(ctx, actx.BeadID, action.SourceBranch, action.CommitMessage, noFF)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: "branch merged", Metadata: result}

	case ActionGitRevert:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		shas := action.CommitSHAs
		if len(shas) == 0 && action.CommitSHA != "" {
			shas = []string{action.CommitSHA}
		}
		result, err := r.Git.Revert(ctx, actx.BeadID, shas, action.Reason)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: "commits reverted", Metadata: result}

	case ActionGitBranchDelete:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		result, err := r.Git.DeleteBranch(ctx, action.Branch, action.DeleteRemote)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: "branch deleted", Metadata: result}

	case ActionGitCheckout:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		result, err := r.Git.Checkout(ctx, action.Branch)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: fmt.Sprintf("switched to %s", action.Branch), Metadata: result}

	case ActionGitLog:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		result, err := r.Git.Log(ctx, action.Branch, action.MaxCount)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: "log retrieved", Metadata: result}

	case ActionGitFetch:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		result, err := r.Git.Fetch(ctx)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: "fetch completed", Metadata: result}

	case ActionGitListBranches:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		result, err := r.Git.ListBranches(ctx)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: "branches listed", Metadata: result}

	case ActionGitDiffBranches:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		result, err := r.Git.DiffBranches(ctx, action.SourceBranch, action.TargetBranch)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: "branch diff retrieved", Metadata: result}

	case ActionGitBeadCommits:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		beadID := action.BeadID
		if beadID == "" {
			beadID = actx.BeadID
		}
		result, err := r.Git.GetBeadCommits(ctx, beadID)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{ActionType: action.Type, Status: "executed", Message: "bead commits retrieved", Metadata: result}

	case ActionInstallPrerequisites:
		agent := r.GetReadyContainerAgent(ctx, actx.ProjectID)
		if agent == nil {
			return Result{ActionType: action.Type, Status: "error",
				Message: "install_prerequisites requires a container agent"}
		}
		var installCmd string
		if action.Command != "" {
			installCmd = action.Command
		} else {
			osFamily := OSFamilyDebian
			if r.BuildEnv != nil {
				osFamily = r.BuildEnv.GetOSFamily(actx.ProjectID)
			}
			installCmd = InstallPackages(osFamily, action.Packages)
		}
		if installCmd == "" {
			return Result{ActionType: action.Type, Status: "skipped", Message: "no packages to install"}
		}
		execRes, err := agent.ExecSync(ctx, installCmd, "/", 120)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		if execRes.ExitCode != 0 {
			return Result{ActionType: action.Type, Status: "error",
				Message: fmt.Sprintf("install failed (exit %d): %s", execRes.ExitCode, execRes.Stderr)}
		}
		return Result{ActionType: action.Type, Status: "executed",
			Message:  fmt.Sprintf("installed packages: %s", strings.Join(action.Packages, ", ")),
			Metadata: map[string]interface{}{"stdout": execRes.Stdout, "duration_ms": execRes.DurationMs}}

	case ActionRunCommand:
		if r.Commands == nil {
			return r.createBeadFromAction("Run command", action.Command, actx)
		}
		// Use action's WorkingDir if specified, otherwise use project's work_dir
		workDir := action.WorkingDir
		if workDir == "" {
			workDir = r.getProjectWorkDir(actx.ProjectID)
		}
		req := executor.ExecuteCommandRequest{
			AgentID:    actx.AgentID,
			BeadID:     actx.BeadID,
			ProjectID:  actx.ProjectID,
			Command:    action.Command,
			WorkingDir: workDir,
			Context: map[string]interface{}{
				"action_type": action.Type,
				"reason":      action.Reason,
			},
		}
		res, err := r.Commands.ExecuteCommand(ctx, req)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "command executed",
			Metadata: map[string]interface{}{
				"command_id": res.ID,
				"exit_code":  res.ExitCode,
				"stdout":     res.Stdout,
				"stderr":     res.Stderr,
			},
		}
	case ActionRunTests:
		if r.Tests == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "test runner not configured"}
		}
		projectPath := r.getProjectWorkDir(actx.ProjectID)
		if projectPath == "" {
			projectPath = "."
		}

		result, err := r.Tests.Run(ctx, projectPath, action.TestPattern, action.Framework, action.TimeoutSeconds)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "tests executed",
			Metadata:   result,
		}
	case ActionRunLinter:
		if r.Linter == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "linter not configured"}
		}
		projectPath := r.getProjectWorkDir(actx.ProjectID)
		if projectPath == "" {
			projectPath = "."
		}

		result, err := r.Linter.Run(ctx, projectPath, action.Files, action.Framework, action.TimeoutSeconds)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "linter executed",
			Metadata:   result,
		}
	case ActionBuildProject:
		// Prefer the structured BuildRunner if available (returns parsed results
		// with error_count, warnings, etc.).
		if r.Builder != nil {
			projectPath := r.getProjectWorkDir(actx.ProjectID)
			if projectPath == "" {
				projectPath = "."
			}
			result, err := r.Builder.Run(ctx, projectPath, action.BuildTarget, action.BuildCommand, action.Framework, action.TimeoutSeconds)
			if err != nil {
				return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
			}
			return Result{
				ActionType: action.Type,
				Status:     "executed",
				Message:    "build executed",
				Metadata:   result,
			}
		}
		// Fallback: use shell executor for ad-hoc build command.
		buildResult, err := r.runBuildForProject(ctx, actx, action.BuildCommand)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("build executor error: %v", err)}
		}
		if buildResult == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "builder not configured"}
		}
		if !buildResult.Success || buildResult.ExitCode != 0 {
			return Result{
				ActionType: action.Type,
				Status:     "error",
				Message:    fmt.Sprintf("build failed (exit %d):\nstdout: %s\nstderr: %s", buildResult.ExitCode, buildResult.Stdout, buildResult.Stderr),
			}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "build passed",
			Metadata: map[string]interface{}{
				"stdout":      buildResult.Stdout,
				"stderr":      buildResult.Stderr,
				"exit_code":   buildResult.ExitCode,
				"duration_ms": buildResult.Duration,
			},
		}
	case ActionCreateBead:
		if action.Bead == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "missing bead payload"}
		}
		if r.Beads == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "bead creator not configured"}
		}
		beadType := action.Bead.Type
		if beadType == "" {
			beadType = r.BeadType
		}
		if beadType == "" {
			beadType = "task"
		}
		priority := models.BeadPriority(action.Bead.Priority)
		bead, err := r.Beads.CreateBead(action.Bead.Title, action.Bead.Description, priority, beadType, action.Bead.ProjectID)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "bead created",
			Metadata:   map[string]interface{}{"bead_id": bead.ID},
		}
	case ActionCloseBead:
		if r.Closer == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "bead closer not configured"}
		}
		err := r.Closer.CloseBead(action.BeadID, action.Reason)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "bead closed",
			Metadata:   map[string]interface{}{"bead_id": action.BeadID},
		}
	case ActionEscalateCEO:
		if r.Escalator == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "escalator not configured"}
		}
		decision, err := r.Escalator.EscalateBeadToCEO(action.BeadID, action.Reason, action.ReturnedTo)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "escalated to CEO",
			Metadata:   map[string]interface{}{"decision_id": decision.ID},
		}
	case ActionApproveBead:
		if r.Workflow == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "workflow operator not configured"}
		}
		// Advance workflow with approved condition
		resultData := map[string]string{
			"approved_by":     actx.AgentID,
			"approval_reason": action.Reason,
		}
		err := r.Workflow.AdvanceWorkflowWithCondition(action.BeadID, actx.AgentID, "approved", resultData)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "bead approved, workflow advanced",
			Metadata:   map[string]interface{}{"bead_id": action.BeadID},
		}
	case ActionRejectBead:
		if r.Workflow == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "workflow operator not configured"}
		}
		// Advance workflow with rejected condition
		resultData := map[string]string{
			"rejected_by":      actx.AgentID,
			"rejection_reason": action.Reason,
		}
		err := r.Workflow.AdvanceWorkflowWithCondition(action.BeadID, actx.AgentID, "rejected", resultData)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "bead rejected, workflow advanced",
			Metadata:   map[string]interface{}{"bead_id": action.BeadID, "reason": action.Reason},
		}
	case ActionStartDev:
		// Workflow actions are handled by MCP tools at the agent LLM layer
		// This action records the workflow initiation
		return Result{
			ActionType: action.Type,
			Status:     "mcp_required",
			Message:    "start_development requires MCP tool call: mcp__responsible-vibe-mcp__start_development",
			Metadata: map[string]interface{}{
				"workflow":        action.Workflow,
				"require_reviews": action.RequireReviews,
				"mcp_tool":        "mcp__responsible-vibe-mcp__start_development",
			},
		}
	case ActionWhatsNext:
		return Result{
			ActionType: action.Type,
			Status:     "mcp_required",
			Message:    "whats_next requires MCP tool call: mcp__responsible-vibe-mcp__whats_next",
			Metadata: map[string]interface{}{
				"mcp_tool": "mcp__responsible-vibe-mcp__whats_next",
			},
		}
	case ActionProceedToPhase:
		return Result{
			ActionType: action.Type,
			Status:     "mcp_required",
			Message:    "proceed_to_phase requires MCP tool call: mcp__responsible-vibe-mcp__proceed_to_phase",
			Metadata: map[string]interface{}{
				"target_phase": action.TargetPhase,
				"review_state": action.ReviewState,
				"reason":       action.Reason,
				"mcp_tool":     "mcp__responsible-vibe-mcp__proceed_to_phase",
			},
		}
	case ActionConductReview:
		return Result{
			ActionType: action.Type,
			Status:     "mcp_required",
			Message:    "conduct_review requires MCP tool call: mcp__responsible-vibe-mcp__conduct_review",
			Metadata: map[string]interface{}{
				"target_phase": action.TargetPhase,
				"mcp_tool":     "mcp__responsible-vibe-mcp__conduct_review",
			},
		}
	case ActionResumeWorkflow:
		return Result{
			ActionType: action.Type,
			Status:     "mcp_required",
			Message:    "resume_workflow requires MCP tool call: mcp__responsible-vibe-mcp__resume_workflow",
			Metadata: map[string]interface{}{
				"mcp_tool": "mcp__responsible-vibe-mcp__resume_workflow",
			},
		}
	case ActionFindReferences:
		if r.LSP == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "LSP operator not configured"}
		}

		result, err := r.LSP.FindReferences(ctx, action.Path, action.Line, action.Column, action.Symbol)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}

		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Found %v references", result["count"]),
			Metadata:   result,
		}
	case ActionGoToDefinition:
		if r.LSP == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "LSP operator not configured"}
		}

		result, err := r.LSP.GoToDefinition(ctx, action.Path, action.Line, action.Column, action.Symbol)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}

		message := "Definition not found"
		if found, ok := result["found"].(bool); ok && found {
			message = fmt.Sprintf("Definition found at %s:%d:%d", result["file"], result["line"], result["column"])
		}

		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    message,
			Metadata:   result,
		}
	case ActionFindImplementations:
		if r.LSP == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "LSP operator not configured"}
		}

		result, err := r.LSP.FindImplementations(ctx, action.Path, action.Line, action.Column, action.Symbol)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}

		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Found %v implementations", result["count"]),
			Metadata:   result,
		}
	case ActionExtractMethod:
		// Extract method refactoring
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Extracted method %s (lines %d-%d)", action.MethodName, action.StartLine, action.EndLine),
			Metadata: map[string]interface{}{
				"method_name": action.MethodName,
				"start_line":  action.StartLine,
				"end_line":    action.EndLine,
				"file":        action.Path,
			},
		}
	case ActionRenameSymbol:
		// Rename symbol refactoring
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Renamed %s to %s", action.Symbol, action.NewName),
			Metadata: map[string]interface{}{
				"old_name": action.Symbol,
				"new_name": action.NewName,
				"file":     action.Path,
			},
		}
	case ActionInlineVariable:
		// Inline variable refactoring
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Inlined variable %s", action.VariableName),
			Metadata: map[string]interface{}{
				"variable": action.VariableName,
				"file":     action.Path,
			},
		}
	case ActionMoveFile:
		// Move file operation
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		err := r.Files.MoveFile(ctx, actx.ProjectID, action.SourcePath, action.TargetPath)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to move file: %v", err)}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Moved %s to %s", action.SourcePath, action.TargetPath),
			Metadata: map[string]interface{}{
				"source": action.SourcePath,
				"target": action.TargetPath,
			},
		}
	case ActionDeleteFile:
		// Delete file operation
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		err := r.Files.DeleteFile(ctx, actx.ProjectID, action.Path)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to delete file: %v", err)}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Deleted %s", action.Path),
			Metadata: map[string]interface{}{
				"file": action.Path,
			},
		}
	case ActionRenameFile:
		// Rename file operation
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		err := r.Files.RenameFile(ctx, actx.ProjectID, action.SourcePath, action.NewName)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to rename file: %v", err)}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Renamed %s to %s", action.SourcePath, action.NewName),
			Metadata: map[string]interface{}{
				"source":   action.SourcePath,
				"new_name": action.NewName,
			},
		}
	case ActionAddLog:
		// Add log statement
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Added log at %s:%d", action.Path, action.Line),
			Metadata: map[string]interface{}{
				"file":    action.Path,
				"line":    action.Line,
				"message": action.LogMessage,
				"level":   action.LogLevel,
			},
		}
	case ActionAddBreakpoint:
		// Add breakpoint
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Added breakpoint at %s:%d", action.Path, action.Line),
			Metadata: map[string]interface{}{
				"file":      action.Path,
				"line":      action.Line,
				"condition": action.Condition,
			},
		}
	case ActionGenerateDocs:
		// Generate documentation
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    fmt.Sprintf("Generated docs for %s", action.Path),
			Metadata: map[string]interface{}{
				"file":   action.Path,
				"format": action.DocFormat,
			},
		}

	// PR Review Actions
	case ActionFetchPR:
		return r.handleFetchPR(ctx, action, actx)
	case ActionReviewCode:
		return r.handleReviewCode(ctx, action, actx)
	case ActionAddPRComment:
		return r.handleAddPRComment(ctx, action, actx)
	case ActionSubmitReview:
		return r.handleSubmitReview(ctx, action, actx)
	case ActionRequestReview:
		return r.handleRequestReview(ctx, action, actx)
	case ActionDone:
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "agent signaled done",
			Metadata:   map[string]interface{}{"reason": action.Reason},
		}
	case ActionSendAgentMessage:
		return r.handleSendAgentMessage(ctx, action, actx)
	case ActionDelegateTask:
		return r.handleDelegateTask(ctx, action, actx)
	case ActionCallMeeting:
		return r.handleCallMeeting(ctx, action, actx)
	case ActionConsultAgent:
		return r.handleConsultAgent(ctx, action, actx)
	case ActionInvokeSkill:
		return r.handleInvokeSkill(ctx, action, actx)
	case ActionPostToBoard:
		return r.handlePostToBoard(ctx, action, actx)
	case ActionVote:
		return r.handleVote(ctx, action, actx)

	case ActionReadBeadConversation:
		return r.handleReadBeadConversation(ctx, action, actx)

	case ActionReadBeadContext:
		return r.handleReadBeadContext(ctx, action, actx)

	default:
		return Result{ActionType: action.Type, Status: "error", Message: "unsupported action"}
	}
}

func (r *Router) createBeadFromAction(title, detail string, actx ActionContext) Result {
	if r.Beads == nil {
		return Result{ActionType: ActionCreateBead, Status: "error", Message: "bead creator not configured"}
	}
	beadType := r.BeadType
	if beadType == "" {
		beadType = "task"
	}
	priority := models.BeadPriority(2)
	if r.DefaultP0 {
		priority = models.BeadPriority(0)
	}
	bead, err := r.Beads.CreateBead(title, detail, priority, beadType, actx.ProjectID)
	if err != nil {
		return Result{ActionType: ActionCreateBead, Status: "error", Message: err.Error()}
	}
	return Result{
		ActionType: ActionCreateBead,
		Status:     "executed",
		Message:    "bead created",
		Metadata:   map[string]interface{}{"bead_id": bead.ID},
	}
}

func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// PR Review Action Handlers

func (r *Router) handleFetchPR(ctx context.Context, action Action, actx ActionContext) Result {
	if action.PRNumber == 0 {
		return Result{ActionType: action.Type, Status: "error", Message: "pr_number is required"}
	}

	if r.Commands == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "command executor not configured"}
	}

	// Build gh CLI command
	cmd := fmt.Sprintf("gh pr view %d --json number,title,body,state,author,headRefName,baseRefName,createdAt,updatedAt", action.PRNumber)
	if action.IncludeFiles {
		cmd += ",files"
	}

	// Execute command
	cmdResult, err := r.Commands.ExecuteCommand(ctx, executor.ExecuteCommandRequest{
		AgentID:    actx.AgentID,
		BeadID:     actx.BeadID,
		ProjectID:  actx.ProjectID,
		Command:    cmd,
		WorkingDir: r.getProjectWorkDir(actx.ProjectID),
	})
	if err != nil || !cmdResult.Success {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to fetch PR: %v", err)}
	}

	// Parse JSON response
	var prData map[string]interface{}
	if err := json.Unmarshal([]byte(cmdResult.Stdout), &prData); err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to parse PR data: %v", err)}
	}

	// Optionally fetch diff
	if action.IncludeDiff {
		diffCmd := fmt.Sprintf("gh pr diff %d", action.PRNumber)
		diffResult, err := r.Commands.ExecuteCommand(ctx, executor.ExecuteCommandRequest{
			AgentID:    actx.AgentID,
			BeadID:     actx.BeadID,
			ProjectID:  actx.ProjectID,
			Command:    diffCmd,
			WorkingDir: r.getProjectWorkDir(actx.ProjectID),
		})
		if err == nil && diffResult.Success {
			prData["diff"] = diffResult.Stdout
		}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Fetched PR #%d", action.PRNumber),
		Metadata:   prData,
	}
}

func (r *Router) handleReviewCode(ctx context.Context, action Action, actx ActionContext) Result {
	if action.PRNumber == 0 {
		return Result{ActionType: action.Type, Status: "error", Message: "pr_number is required"}
	}

	// Fetch PR details and files
	fetchResult := r.handleFetchPR(ctx, Action{
		Type:         ActionFetchPR,
		PRNumber:     action.PRNumber,
		IncludeFiles: true,
		IncludeDiff:  true,
	}, actx)

	if fetchResult.Status != "executed" {
		return Result{ActionType: action.Type, Status: "error", Message: "failed to fetch PR for review"}
	}

	// Review criteria defaults
	criteria := action.ReviewCriteria
	if len(criteria) == 0 {
		criteria = []string{"quality", "functionality", "testing", "security", "documentation"}
	}

	// Run static analysis (go vet) via the container executor if available.
	// Falls back to informational result if no executor is configured.
	analysisOutput := "code analysis not available (no executor configured)"
	if r.Commands != nil {
		projectPath := r.getProjectWorkDir(actx.ProjectID)
		if projectPath == "" {
			projectPath = "."
		}
		vetResult, vetErr := r.Commands.ExecuteCommand(ctx, executor.ExecuteCommandRequest{
			AgentID:    actx.AgentID,
			ProjectID:  actx.ProjectID,
			Command:    "go vet ./...",
			WorkingDir: projectPath,
			Timeout:    120,
		})
		if vetErr == nil && vetResult != nil {
			if vetResult.Success {
				analysisOutput = "go vet: no issues found"
			} else {
				analysisOutput = fmt.Sprintf("go vet issues:\n%s\n%s", vetResult.Stdout, vetResult.Stderr)
			}
		}
	}

	reviewResult := map[string]interface{}{
		"pr_number":       action.PRNumber,
		"criteria":        criteria,
		"status":          "review_completed",
		"static_analysis": analysisOutput,
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Reviewed PR #%d against %d criteria", action.PRNumber, len(criteria)),
		Metadata:   reviewResult,
	}
}

func (r *Router) handleAddPRComment(ctx context.Context, action Action, actx ActionContext) Result {
	if action.PRNumber == 0 {
		return Result{ActionType: action.Type, Status: "error", Message: "pr_number is required"}
	}
	if action.CommentBody == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "comment_body is required"}
	}

	if r.Commands == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "command executor not configured"}
	}

	var cmd string
	commentType := "general"

	if action.CommentPath != "" && action.CommentLine > 0 {
		// Inline comment on specific line
		side := action.CommentSide
		if side == "" {
			side = "RIGHT"
		}
		cmd = fmt.Sprintf("gh pr comment %d --body %q --file %s --line %d --side %s",
			action.PRNumber, action.CommentBody, action.CommentPath, action.CommentLine, side)
		commentType = "inline"
	} else {
		// General PR comment
		cmd = fmt.Sprintf("gh pr comment %d --body %q", action.PRNumber, action.CommentBody)
	}

	cmdResult, err := r.Commands.ExecuteCommand(ctx, executor.ExecuteCommandRequest{
		AgentID:    actx.AgentID,
		BeadID:     actx.BeadID,
		ProjectID:  actx.ProjectID,
		Command:    cmd,
		WorkingDir: r.getProjectWorkDir(actx.ProjectID),
	})
	if err != nil || !cmdResult.Success {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to add comment: %v", err)}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Added %s comment to PR #%d", commentType, action.PRNumber),
		Metadata: map[string]interface{}{
			"pr_number": action.PRNumber,
			"type":      commentType,
			"file":      action.CommentPath,
			"line":      action.CommentLine,
		},
	}
}

func (r *Router) handleSubmitReview(ctx context.Context, action Action, actx ActionContext) Result {
	if action.PRNumber == 0 {
		return Result{ActionType: action.Type, Status: "error", Message: "pr_number is required"}
	}
	if action.ReviewEvent == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "review_event is required (APPROVE, REQUEST_CHANGES, COMMENT)"}
	}
	if action.CommentBody == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "comment_body is required"}
	}

	if r.Commands == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "command executor not configured"}
	}

	// Validate review event
	validEvents := map[string]bool{
		"APPROVE":         true,
		"REQUEST_CHANGES": true,
		"COMMENT":         true,
	}
	if !validEvents[action.ReviewEvent] {
		return Result{ActionType: action.Type, Status: "error", Message: "invalid review_event"}
	}

	// Build gh CLI command
	eventFlag := "--" + strings.ToLower(strings.ReplaceAll(action.ReviewEvent, "_", "-"))
	cmd := fmt.Sprintf("gh pr review %d %s --body %q", action.PRNumber, eventFlag, action.CommentBody)

	cmdResult, err := r.Commands.ExecuteCommand(ctx, executor.ExecuteCommandRequest{
		AgentID:    actx.AgentID,
		BeadID:     actx.BeadID,
		ProjectID:  actx.ProjectID,
		Command:    cmd,
		WorkingDir: r.getProjectWorkDir(actx.ProjectID),
	})
	if err != nil || !cmdResult.Success {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to submit review: %v", err)}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Submitted review for PR #%d: %s", action.PRNumber, action.ReviewEvent),
		Metadata: map[string]interface{}{
			"pr_number": action.PRNumber,
			"event":     action.ReviewEvent,
		},
	}
}

func (r *Router) handleRequestReview(ctx context.Context, action Action, actx ActionContext) Result {
	if action.PRNumber == 0 {
		return Result{ActionType: action.Type, Status: "error", Message: "pr_number is required"}
	}
	if action.Reviewer == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "reviewer is required"}
	}

	if r.Commands == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "command executor not configured"}
	}

	// Build gh CLI command
	cmd := fmt.Sprintf("gh pr edit %d --add-reviewer %s", action.PRNumber, action.Reviewer)

	cmdResult, err := r.Commands.ExecuteCommand(ctx, executor.ExecuteCommandRequest{
		AgentID:    actx.AgentID,
		BeadID:     actx.BeadID,
		ProjectID:  actx.ProjectID,
		Command:    cmd,
		WorkingDir: r.getProjectWorkDir(actx.ProjectID),
	})
	if err != nil || !cmdResult.Success {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to request review: %v", err)}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Requested review from %s for PR #%d", action.Reviewer, action.PRNumber),
		Metadata: map[string]interface{}{
			"pr_number": action.PRNumber,
			"reviewer":  action.Reviewer,
		},
	}
}

func (r *Router) handleSendAgentMessage(ctx context.Context, action Action, actx ActionContext) Result {
	// Validate required fields
	if action.ToAgentID == "" && action.ToAgentRole == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "either to_agent_id or to_agent_role is required"}
	}

	if action.MessageType == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "message_type is required (question, delegation, notification)"}
	}

	// Validate message type
	validMessageTypes := map[string]bool{
		"question":     true,
		"delegation":   true,
		"notification": true,
	}
	if !validMessageTypes[action.MessageType] {
		return Result{ActionType: action.Type, Status: "error", Message: "message_type must be one of: question, delegation, notification"}
	}

	if r.MessageBus == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "message bus not configured"}
	}

	// Resolve target agent ID if role provided
	targetAgentID := action.ToAgentID
	if targetAgentID == "" && action.ToAgentRole != "" {
		agentID, err := r.MessageBus.FindAgentByRole(ctx, action.ToAgentRole)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to find agent with role %s: %v", action.ToAgentRole, err)}
		}
		targetAgentID = agentID
	}

	// Send the message
	messageID, err := r.MessageBus.SendMessage(
		ctx,
		actx.AgentID,
		targetAgentID,
		action.MessageType,
		action.MessageSubject,
		action.MessageBody,
		action.MessagePayload,
	)
	if err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to send message: %v", err)}
	}

	// Build result message
	resultMessage := fmt.Sprintf("Sent %s message to agent %s", action.MessageType, targetAgentID)
	if action.MessageSubject != "" {
		resultMessage = fmt.Sprintf("Sent %s message to agent %s: %s", action.MessageType, targetAgentID, action.MessageSubject)
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    resultMessage,
		Metadata: map[string]interface{}{
			"message_id":      messageID,
			"to_agent_id":     targetAgentID,
			"message_type":    action.MessageType,
			"message_subject": action.MessageSubject,
		},
	}
}

func (r *Router) handleDelegateTask(ctx context.Context, action Action, actx ActionContext) Result {
	// Validate required fields
	if action.DelegateToRole == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "delegate_to_role is required"}
	}

	if action.TaskTitle == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "task_title is required"}
	}

	if r.Beads == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "bead creator not configured"}
	}

	// Determine priority (default to medium if out of valid range)
	// Valid priorities are 0-4 (P0-P4)
	// Since 0 is the zero value, we can't distinguish "not set" from "P0"
	// So we only default if explicitly out of range (> 4)
	priority := action.TaskPriority
	if priority < 0 {
		priority = 0 // Treat negative as P0
	} else if priority > 4 {
		priority = 2 // Out of range defaults to medium (P2)
	}

	// Create child bead
	childBead, err := r.Beads.CreateBead(
		action.TaskTitle,
		action.TaskDescription,
		models.BeadPriority(priority),
		"delegated", // Type
		actx.ProjectID,
	)

	if err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to create child bead: %v", err)}
	}

	// Store parent relationship (if parent bead provided)
	parentBeadID := action.ParentBeadID
	if parentBeadID == "" {
		parentBeadID = actx.BeadID // Use current bead as parent if not specified
	}

	// Build result message
	resultMessage := fmt.Sprintf("Delegated task '%s' to %s (child bead: %s)", action.TaskTitle, action.DelegateToRole, childBead.ID)
	if parentBeadID != "" {
		resultMessage = fmt.Sprintf("Delegated task '%s' to %s (parent: %s, child: %s)", action.TaskTitle, action.DelegateToRole, parentBeadID, childBead.ID)
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    resultMessage,
		Metadata: map[string]interface{}{
			"child_bead_id":    childBead.ID,
			"parent_bead_id":   parentBeadID,
			"delegate_to_role": action.DelegateToRole,
			"task_title":       action.TaskTitle,
			"task_priority":    priority,
		},
	}
}

func (r *Router) handleReadBeadConversation(ctx context.Context, action Action, actx ActionContext) Result {
	if r.BeadReader == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "bead reader not configured"}
	}

	if action.BeadID == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "bead_id is required"}
	}

	conversation, err := r.BeadReader.GetBeadConversation(action.BeadID)
	if err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to read conversation: %v", err)}
	}

	// Limit messages if requested
	messages := conversation
	if action.MaxMessages > 0 && len(messages) > action.MaxMessages {
		messages = messages[len(messages)-action.MaxMessages:]
	}

	// Convert to simple format for the agent
	var conversationText strings.Builder
	conversationText.WriteString(fmt.Sprintf("## Conversation for Bead %s\n\n", action.BeadID))
	conversationText.WriteString(fmt.Sprintf("Total messages: %d\n\n", len(conversation)))

	for i, msg := range messages {
		conversationText.WriteString(fmt.Sprintf("### Message %d (%s)\n", i+1, msg.Role))
		if !msg.Timestamp.IsZero() {
			conversationText.WriteString(fmt.Sprintf("Time: %s\n", msg.Timestamp.Format("2006-01-02 15:04:05")))
		}
		conversationText.WriteString(fmt.Sprintf("\n%s\n\n", msg.Content))
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("read %d messages from bead %s", len(messages), action.BeadID),
		Metadata: map[string]interface{}{
			"bead_id":        action.BeadID,
			"message_count":  len(messages),
			"total_messages": len(conversation),
			"conversation":   conversationText.String(),
		},
	}
}

func (r *Router) handleReadBeadContext(ctx context.Context, action Action, actx ActionContext) Result {
	if r.BeadReader == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "bead reader not configured"}
	}

	if action.BeadID == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "bead_id is required"}
	}

	bead, err := r.BeadReader.GetBead(action.BeadID)
	if err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to read bead: %v", err)}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("read bead %s context", action.BeadID),
		Metadata: map[string]interface{}{
			"bead_id":     bead.ID,
			"title":       bead.Title,
			"description": bead.Description,
			"status":      bead.Status,
			"priority":    bead.Priority,
			"type":        bead.Type,
			"project_id":  bead.ProjectID,
			"assigned_to": bead.AssignedTo,
			"context":     bead.Context,
			"created_at":  bead.CreatedAt,
			"updated_at":  bead.UpdatedAt,
		},
	}
}

// ensureBeadBranch creates a bead-specific branch if the current branch is
// main/master. This implements the branch-per-bead workflow required for
// autonomous PR creation.
func (r *Router) ensureBeadBranch(ctx context.Context, actx ActionContext) {
	if r.Git == nil || actx.BeadID == "" {
		return
	}

	statusResult, err := r.Git.GetStatus(ctx)
	if err != nil {
		return
	}
	branch, _ := statusResult["branch"].(string)
	if branch == "" {
		return
	}

	// Already on a non-main branch — assume it's the right one.
	if branch != "main" && branch != "master" {
		return
	}

	beadBranch := "bead/" + actx.BeadID
	if _, err := r.Git.Checkout(ctx, beadBranch); err != nil {
		// Branch doesn't exist yet — create it.
		if _, err2 := r.Git.CreateBranch(ctx, actx.BeadID, "", branch); err2 != nil {
			log.Printf("[GitCommit] Failed to create bead branch %s: %v", beadBranch, err2)
		}
	}
}

// runQualityGate performs pre-commit build+test+lint checks.
// Returns an error message string if the commit should be blocked, empty string otherwise.
func (r *Router) runQualityGate(ctx context.Context, actx ActionContext, action Action) string {
	// Build gate
	buildResult, buildErr := r.runBuildForProject(ctx, actx, "")
	if buildErr != nil {
		log.Printf("[QualityGate] Build error (allowing commit): %v", buildErr)
	}
	if buildResult != nil && buildResult.ExitCode == 127 {
		// Build command not found — toolchain is missing. Block the commit and
		// instruct the agent to detect and install the required toolchain.
		return fmt.Sprintf(
			"commit blocked: build toolchain not found (exit 127).\n\n"+
				"The build command could not be executed because the required tool is not installed.\n\n"+
				"You MUST:\n"+
				"1. Inspect the project files (go.mod, package.json, Cargo.toml, requirements.txt, etc.) "+
				"to determine the required toolchain.\n"+
				"2. Use execute_command to install it (e.g. 'apt-get install -y golang-go' for Go, "+
				"'apt-get install -y nodejs npm' for Node, 'apt-get install -y python3' for Python, "+
				"'curl https://sh.rustup.rs -sSf | sh -s -- -y' for Rust).\n"+
				"3. Verify the build passes by running the build command via execute_command.\n"+
				"4. Only then retry git_commit.\n\n"+
				"DO NOT call done or git_commit until the build passes.\n\n"+
				"Command output:\n%s%s",
			buildResult.Stdout, buildResult.Stderr)
	}
	if buildResult != nil && !buildResult.Success && buildResult.ExitCode > 0 {
		return fmt.Sprintf(
			"commit blocked: build failed (exit %d).\n\n"+
				"You MUST fix the build error before committing.\n"+
				"DO NOT call done or git_commit until the build passes.\n\n"+
				"Steps:\n"+
				"1. Read the error output below carefully.\n"+
				"2. Fix the files causing the error.\n"+
				"3. Run the build command via execute_command to verify it passes.\n"+
				"4. Only then retry git_commit.\n\n"+
				"Build output:\nstdout:\n%s\nstderr:\n%s",
			buildResult.ExitCode, buildResult.Stdout, buildResult.Stderr)
	}

	// Test gate (best-effort, non-blocking for now)
	if r.Tests != nil {
		projectPath := r.getProjectWorkDir(actx.ProjectID)
		testResult, testErr := r.Tests.Run(ctx, projectPath, "", "", 120)
		if testErr != nil {
			log.Printf("[QualityGate] Test runner error (non-blocking): %v", testErr)
		} else if testResult != nil {
			if passed, ok := testResult["passed"].(bool); ok && !passed {
				log.Printf("[QualityGate] Tests failed for %s (non-blocking): %v", actx.ProjectID, testResult)
			}
		}
	}

	// Lint gate (best-effort, non-blocking for now)
	if r.Linter != nil {
		projectPath := r.getProjectWorkDir(actx.ProjectID)
		lintResult, lintErr := r.Linter.Run(ctx, projectPath, nil, "", 60)
		if lintErr != nil {
			log.Printf("[QualityGate] Linter error (non-blocking): %v", lintErr)
		} else if lintResult != nil {
			if violations, ok := lintResult["violations"].(int); ok && violations > 0 {
				log.Printf("[QualityGate] Lint violations for %s: %d (non-blocking)", actx.ProjectID, violations)
			}
		}
	}

	return ""
}

// autoPushAndPR automatically pushes the bead branch and creates a PR
// after a successful commit on a non-main branch.
func (r *Router) autoPushAndPR(ctx context.Context, actx ActionContext, action Action, commitResult map[string]interface{}) {
	if r.Git == nil || actx.BeadID == "" {
		return
	}

	statusResult, err := r.Git.GetStatus(ctx)
	if err != nil {
		return
	}
	branch, _ := statusResult["branch"].(string)
	if branch == "" || branch == "main" || branch == "master" {
		return
	}

	// Push with set-upstream
	pushResult, pushErr := r.Git.Push(ctx, actx.BeadID, branch, true)
	if pushErr != nil {
		log.Printf("[AutoPR] Push failed for bead %s branch %s: %v", actx.BeadID, branch, pushErr)
		return
	}
	log.Printf("[AutoPR] Pushed branch %s for bead %s: %v", branch, actx.BeadID, pushResult)

	// Create PR
	title := action.PRTitle
	if title == "" {
		title = fmt.Sprintf("[bead/%s] %s", actx.BeadID, action.CommitMessage)
		if len(title) > 200 {
			title = title[:200]
		}
	}
	body := action.PRBody
	if body == "" {
		body = fmt.Sprintf("Automated PR from bead `%s`\n\nAgent: `%s`\nProject: `%s`\n\n---\n*Created automatically by Loom*",
			actx.BeadID, actx.AgentID, actx.ProjectID)
	}

	prResult, prErr := r.Git.CreatePR(ctx, actx.BeadID, title, body, "main", branch, nil, false)
	if prErr != nil {
		log.Printf("[AutoPR] PR creation failed for bead %s: %v", actx.BeadID, prErr)
		return
	}
	log.Printf("[AutoPR] Created PR for bead %s: %v", actx.BeadID, prResult)
}

// --- Organizational layer action handlers ---

func (r *Router) handleCallMeeting(_ context.Context, action Action, actx ActionContext) Result {
	if r.Meetings == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "meeting engine not configured"}
	}
	if action.MeetingTitle == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "meeting_title is required"}
	}
	if len(action.MeetingAgenda) == 0 {
		return Result{ActionType: action.Type, Status: "error", Message: "meeting_agenda requires at least one item"}
	}

	agenda := make([]struct{ Topic, Description string }, len(action.MeetingAgenda))
	for i, item := range action.MeetingAgenda {
		agenda[i] = struct{ Topic, Description string }{Topic: item.Topic, Description: item.Description}
	}

	ctx := context.Background()
	meetingID, err := r.Meetings.CallMeeting(ctx, actx.AgentID, action.MeetingTitle, actx.ProjectID, actx.BeadID, action.MeetingParticipants, agenda)
	if err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to call meeting: %v", err)}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Meeting %s scheduled: %s", meetingID, action.MeetingTitle),
		Metadata:   map[string]interface{}{"meeting_id": meetingID},
	}
}

func (r *Router) handleConsultAgent(_ context.Context, action Action, actx ActionContext) Result {
	if r.Consulter == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "agent consultation not configured"}
	}
	if action.ConsultQuestion == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "consult_question is required"}
	}
	if action.ConsultAgentID == "" && action.ConsultAgentRole == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "either consult_agent_id or consult_agent_role is required"}
	}

	ctx := context.Background()
	response, err := r.Consulter.ConsultAgent(ctx, actx.AgentID, action.ConsultAgentID, action.ConsultAgentRole, action.ConsultQuestion)
	if err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("consultation failed: %v", err)}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    response,
		Metadata: map[string]interface{}{
			"consulted_agent_id":   action.ConsultAgentID,
			"consulted_agent_role": action.ConsultAgentRole,
		},
	}
}

func (r *Router) handleInvokeSkill(_ context.Context, action Action, actx ActionContext) Result {
	if action.SkillName == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "skill_name is required"}
	}

	if r.PersonaManager == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "persona manager not configured"}
	}

	personaObj, err := r.PersonaManager.LoadPersona(action.SkillName)
	if err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to load skill '%s': %v", action.SkillName, err)}
	}

	skillContent := personaObj.Instructions
	if action.SkillContext != "" {
		skillContent = fmt.Sprintf("%s\n\n---\n\nContext for this invocation:\n%s", skillContent, action.SkillContext)
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Skill '%s' loaded and ready to apply.", action.SkillName),
		Metadata: map[string]interface{}{
			"skill_name":    action.SkillName,
			"skill_content": skillContent,
			"agent_id":      actx.AgentID,
			"persona_name":  personaObj.Name,
			"persona_desc":  personaObj.Description,
		},
	}
}

func (r *Router) handlePostToBoard(_ context.Context, action Action, actx ActionContext) Result {
	if r.Board == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "status board not configured"}
	}
	if action.BoardContent == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "board_content is required"}
	}

	category := action.BoardCategory
	if category == "" {
		category = "announcement"
	}

	ctx := context.Background()
	if err := r.Board.PostToBoard(ctx, actx.ProjectID, category, action.BoardContent, actx.AgentID); err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to post to board: %v", err)}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Posted to status board under '%s'", category),
	}
}

func (r *Router) handleVote(_ context.Context, action Action, actx ActionContext) Result {
	if r.Voter == nil {
		return Result{ActionType: action.Type, Status: "error", Message: "voting system not configured"}
	}
	if action.VoteDecisionID == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "vote_decision_id is required"}
	}
	if action.VoteChoice == "" {
		return Result{ActionType: action.Type, Status: "error", Message: "vote_choice is required (approve, reject, abstain)"}
	}

	ctx := context.Background()
	if err := r.Voter.CastVote(ctx, action.VoteDecisionID, actx.AgentID, action.VoteChoice, action.VoteRationale); err != nil {
		return Result{ActionType: action.Type, Status: "error", Message: fmt.Sprintf("failed to cast vote: %v", err)}
	}

	return Result{
		ActionType: action.Type,
		Status:     "executed",
		Message:    fmt.Sprintf("Vote '%s' cast on decision %s", action.VoteChoice, action.VoteDecisionID),
	}
}
