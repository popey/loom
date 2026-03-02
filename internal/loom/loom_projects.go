package loom

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/jordanhubbard/loom/internal/eventbus"
	"github.com/jordanhubbard/loom/internal/project"
	"github.com/jordanhubbard/loom/pkg/models"
)

func (a *Loom) GetProjectManager() *project.Manager {
	return a.projectManager
}
func (a *Loom) GetProject(projectID string) (*models.Project, error) {
	return a.projectManager.GetProject(projectID)
}
func (a *Loom) ReloadProjectBeads(ctx context.Context, projectID string) (int, error) {
	_, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return 0, fmt.Errorf("project not found: %s", projectID)
	}

	beadsPath := a.beadsManager.GetProjectBeadsPath(projectID)
	if beadsPath == "" {
		return 0, fmt.Errorf("no beads path configured for project %s", projectID)
	}

	a.beadsManager.ClearProjectBeads(projectID)

	if err := a.beadsManager.LoadBeadsFromGit(ctx, projectID, beadsPath); err != nil {
		return 0, fmt.Errorf("reload failed: %w", err)
	}

	all, _ := a.beadsManager.ListBeads(map[string]interface{}{"project_id": projectID})
	return len(all), nil
}
func (a *Loom) GetProjectWorkDir(projectID string) string {
	p, err := a.projectManager.GetProject(projectID)
	if err != nil || p == nil {
		return ""
	}
	return p.WorkDir
}
func (a *Loom) ListProjectIDs() []string {
	projects := a.projectManager.ListProjects()
	ids := make([]string, 0, len(projects))
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	return ids
}
func (a *Loom) CreateProject(name, gitRepo, branch, beadsPath string, ctxMap map[string]string) (*models.Project, error) {
	p, err := a.projectManager.CreateProject(name, gitRepo, branch, beadsPath, ctxMap)
	if err != nil {
		return nil, err
	}
	p.BeadsPath = normalizeBeadsPath(p.BeadsPath)
	p.GitAuthMethod = normalizeGitAuthMethod(p.GitRepo, p.GitAuthMethod)
	_ = a.ensureDefaultAgents(context.Background(), p.ID)
	if a.database != nil {
		_ = a.database.UpsertProject(p)
	}
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeProjectCreated,
			Source:    "project-manager",
			ProjectID: p.ID,
			Data: map[string]interface{}{
				"project_id": p.ID,
				"name":       p.Name,
			},
		})
	}
	return p, nil
}
func (a *Loom) CheckProjectReadiness(ctx context.Context, projectID string) (bool, []string) {
	if projectID == "" {
		return true, nil
	}

	now := time.Now()
	a.readinessMu.Lock()
	if cached, ok := a.readinessCache[projectID]; ok {
		if now.Sub(cached.checkedAt) < readinessCacheTTL {
			issues := append([]string(nil), cached.issues...)
			ready := cached.ready
			a.readinessMu.Unlock()
			return ready, issues
		}
	}
	a.readinessMu.Unlock()

	project, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return false, []string{err.Error()}
	}

	issues := []string{}
	publicKey := ""
	if project.GitRepo != "" && project.GitRepo != "." {
		if project.GitAuthMethod == "" {
			project.GitAuthMethod = normalizeGitAuthMethod(project.GitRepo, project.GitAuthMethod)
		}
		if project.GitAuthMethod == models.GitAuthSSH {
			key, err := a.gitopsManager.EnsureProjectSSHKey(project.ID)
			if err != nil {
				issues = append(issues, fmt.Sprintf("ssh key generation failed: %v", err))
			} else {
				publicKey = key
			}
			if !isSSHRepo(project.GitRepo) {
				issues = append(issues, "git repo is not using SSH (update git_repo to an SSH URL or set git_auth_method)")
			}
		}
		if err := a.gitopsManager.CheckRemoteAccess(ctx, project); err != nil {
			issues = append(issues, fmt.Sprintf("git remote access failed: %v", err))
		}
	}

	beadsPath := project.BeadsPath
	if project.GitRepo != "" && project.GitRepo != "." {
		beadsPath = filepath.Join(a.gitopsManager.GetProjectWorkDir(project.ID), project.BeadsPath)
	}
	if !beadsPathExists(beadsPath) {
		issues = append(issues, fmt.Sprintf("beads path missing: %s", beadsPath))
	}

	ready := len(issues) == 0
	a.readinessMu.Lock()
	a.readinessCache[projectID] = projectReadinessState{ready: ready, issues: issues, checkedAt: now}
	a.readinessMu.Unlock()

	if !ready {
		// Attempt self-healing before filing a bead.
		healed := a.attemptSelfHeal(ctx, project, issues)
		if healed {
			log.Printf("[Readiness] Self-healed issues for project %s, rechecking", projectID)
			a.readinessMu.Lock()
			delete(a.readinessCache, projectID)
			a.readinessMu.Unlock()
			return a.CheckProjectReadiness(ctx, projectID)
		}
		a.maybeFileReadinessBead(project, issues, publicKey)
	}

	return ready, issues
}
func (a *Loom) DiagnoseProject(ctx context.Context, projectID string) map[string]interface{} {
	diag := map[string]interface{}{
		"project_id": projectID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	ready, issues := a.CheckProjectReadiness(ctx, projectID)
	diag["ready"] = ready
	diag["issues"] = issues

	// Check container status if orchestrator is available
	if a.containerOrchestrator != nil {
		agent, err := a.containerOrchestrator.GetAgent(projectID)
		if err != nil {
			diag["container_status"] = "not_running"
			diag["container_error"] = err.Error()
		} else if agent != nil {
			if healthErr := agent.Health(ctx); healthErr != nil {
				diag["container_status"] = "unhealthy"
				diag["container_error"] = healthErr.Error()
			} else {
				diag["container_status"] = "healthy"
				status, _ := agent.Status(ctx)
				if status != nil {
					diag["agent_status"] = status
				}
			}
		}
	}

	// Check build env readiness
	if a.actionRouter != nil && a.actionRouter.BuildEnv != nil {
		diag["build_env_ready"] = a.actionRouter.BuildEnv.IsReady(projectID)
		diag["os_family"] = a.actionRouter.BuildEnv.GetOSFamily(projectID).String()
	}

	return diag
}
func (a *Loom) GetProjectGitPublicKey(projectID string) (string, error) {
	project, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return "", err
	}
	if project.GitAuthMethod != models.GitAuthSSH {
		return "", fmt.Errorf("project %s is not configured for ssh auth", projectID)
	}
	return a.gitopsManager.GetProjectPublicKey(projectID)
}
func (a *Loom) RotateProjectGitKey(projectID string) (string, error) {
	project, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return "", err
	}
	if project.GitAuthMethod != models.GitAuthSSH {
		return "", fmt.Errorf("project %s is not configured for ssh auth", projectID)
	}
	return a.gitopsManager.RotateProjectSSHKey(projectID)
}
func (a *Loom) AssignAgentToProject(agentID, projectID string) error {
	agent, err := a.agentManager.GetAgent(agentID)
	if err != nil {
		return err
	}
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return err
	}

	if agent.ProjectID != "" && agent.ProjectID != projectID {
		_ = a.projectManager.RemoveAgentFromProject(agent.ProjectID, agentID)
		a.PersistProject(agent.ProjectID)
	}

	if err := a.agentManager.UpdateAgentProject(agentID, projectID); err != nil {
		return err
	}
	_ = a.projectManager.AddAgentToProject(projectID, agentID)
	a.PersistProject(projectID)

	return nil
}
func (a *Loom) UnassignAgentFromProject(agentID, projectID string) error {
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return err
	}
	if err := a.projectManager.RemoveAgentFromProject(projectID, agentID); err != nil {
		return err
	}
	_ = a.agentManager.UpdateAgentProject(agentID, "")
	a.PersistProject(projectID)
	return nil
}
func (a *Loom) PersistProject(projectID string) {
	if a.database == nil {
		return
	}
	p, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return
	}
	_ = a.database.UpsertProject(p)
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeProjectUpdated,
			Source:    "project-manager",
			ProjectID: p.ID,
			Data: map[string]interface{}{
				"project_id": p.ID,
				"name":       p.Name,
			},
		})
	}
}
func (a *Loom) DeleteProject(projectID string) error {
	if err := a.projectManager.DeleteProject(projectID); err != nil {
		return err
	}
	if a.database != nil {
		_ = a.database.DeleteProject(projectID)
	}
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeProjectDeleted,
			Source:    "project-manager",
			ProjectID: projectID,
			Data: map[string]interface{}{
				"project_id": projectID,
			},
		})
	}
	return nil
}
func (a *Loom) WakeProject(projectID string) {
	if a.taskExecutor != nil {
		a.taskExecutor.WakeProject(projectID)
	}
}
func (a *Loom) GetProjectIdle(projectID string, duration time.Duration) (bool, error) {
	// TODO: Implement project idle checking
	return false, nil
}
