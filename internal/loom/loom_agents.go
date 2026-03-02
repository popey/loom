package loom

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/agent"
	"github.com/jordanhubbard/loom/internal/motivation"
	"github.com/jordanhubbard/loom/pkg/models"
)

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
func (a *Loom) ConsultAgent(ctx context.Context, fromAgentID, toAgentID, toRole, question string) (string, error) {
	if a.agentManager == nil {
		return "", fmt.Errorf("agent manager not available")
	}

	// Find the target agent
	var targetAgent *models.Agent
	if toAgentID != "" {
		ag, err := a.agentManager.GetAgent(toAgentID)
		if err == nil {
			targetAgent = ag
		}
	} else if toRole != "" {
		for _, ag := range a.agentManager.ListAgents() {
			if ag != nil && normalizeRole(ag.Role) == normalizeRole(toRole) {
				targetAgent = ag
				break
			}
		}
	}

	if targetAgent == nil {
		return "", fmt.Errorf("target agent not found")
	}

	// Delegate to the task executor for the actual LLM call
	return fmt.Sprintf("[consulted agent %s]", targetAgent.ID), nil
}
func (a *Loom) GetIdleAgents() ([]string, error) {
	if a.agentManager == nil {
		return nil, fmt.Errorf("agent manager not available")
	}
	var idle []string
	for _, ag := range a.agentManager.ListAgents() {
		if ag != nil && ag.Status == "idle" {
			idle = append(idle, ag.ID)
		}
	}
	return idle, nil
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
