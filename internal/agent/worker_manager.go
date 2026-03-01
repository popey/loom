package agent

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/weavedev/loom/internal/database"
	"github.com/weavedev/loom/pkg/models"
)

// WorkerManager manages the lifecycle of worker agents.
type WorkerManager struct {
	db              *database.Database
	agents          map[string]*models.Agent
	agentsMutex     sync.RWMutex
	workerProviders map[string]WorkerProvider
}

// WorkerProvider is an interface for creating worker agents.
type WorkerProvider interface {
	CreateAgent(name, role string) (*models.Agent, error)
}

// NewWorkerManager creates a new WorkerManager.
func NewWorkerManager(db *database.Database) *WorkerManager {
	return &WorkerManager{
		db:              db,
		agents:          make(map[string]*models.Agent),
		workerProviders: make(map[string]WorkerProvider),
	}
}

// RegisterWorkerProvider registers a worker provider for a given role.
func (wm *WorkerManager) RegisterWorkerProvider(role string, provider WorkerProvider) {
	wm.workerProviders[role] = provider
}

// RegisterAgentsForProject creates and registers agents for all positions in the org chart.
func (wm *WorkerManager) RegisterAgentsForProject(orgChart *models.OrgChart) error {
	if orgChart == nil {
		return fmt.Errorf("org chart is nil")
	}

	wm.agentsMutex.Lock()
	defer wm.agentsMutex.Unlock()

	// Iterate through all positions in the org chart
	for i := range orgChart.Positions {
		position := &orgChart.Positions[i]
		// Skip if agent already assigned
		if len(position.AgentIDs) > 0 {
			log.Printf("Position %s already has agents assigned: %v", position.ID, position.AgentIDs)
			continue
		}

		// Get the worker provider for this role
		provider, ok := wm.workerProviders[position.RoleName]
		if !ok {
			log.Printf("No worker provider registered for role: %s", position.RoleName)
			continue
		}

		// Create the agent
		agentName := fmt.Sprintf("agent-%d-%s", time.Now().UnixNano(), position.RoleName)
		agent, err := provider.CreateAgent(agentName, position.RoleName)
		if err != nil {
			log.Printf("Failed to create agent for position %s (role %s): %v", position.ID, position.RoleName, err)
			continue
		}

		// Store the agent
		wm.agents[agent.ID] = agent
		log.Printf("Created agent %s for position %s (role %s)", agent.ID, position.ID, position.RoleName)

		// Assign the agent to the position in the org chart
		position.AgentIDs = append(position.AgentIDs, agent.ID)
		log.Printf("Assigned agent %s to position %s", agent.ID, position.ID)
	}

	// Persist the org chart to the database
	if err := wm.db.UpsertOrgChart(orgChart); err != nil {
		return fmt.Errorf("failed to persist org chart: %w", err)
	}

	log.Printf("Successfully registered agents for project and persisted org chart")
	return nil
}

// GetAgent retrieves an agent by ID.
func (wm *WorkerManager) GetAgent(agentID string) (*models.Agent, bool) {
	wm.agentsMutex.RLock()
	defer wm.agentsMutex.RUnlock()
	agent, ok := wm.agents[agentID]
	return agent, ok
}

// GetAgents returns all registered agents.
func (wm *WorkerManager) GetAgents() []*models.Agent {
	wm.agentsMutex.RLock()
	defer wm.agentsMutex.RUnlock()
	agents := make([]*models.Agent, 0, len(wm.agents))
	for _, agent := range wm.agents {
		agents = append(agents, agent)
	}
	return agents
}

// LoadAgentsFromOrgChart loads agents from the org chart into the manager.
func (wm *WorkerManager) LoadAgentsFromOrgChart(orgChart *models.OrgChart) error {
	wm.agentsMutex.Lock()
	defer wm.agentsMutex.Unlock()

	for _, position := range orgChart.Positions {
		for _, agentID := range position.AgentIDs {
			// Load agent from database if not already loaded
			if _, ok := wm.agents[agentID]; !ok {
				agent, err := wm.db.GetAgent(agentID)
				if err != nil {
					log.Printf("Failed to load agent %s: %v", agentID, err)
					continue
				}
				wm.agents[agentID] = agent
				log.Printf("Loaded agent %s from database", agentID)
			}
		}
	}

	return nil
}
