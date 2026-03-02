package loom

import (
	"log"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/pkg/models"
)

func (a *Loom) resetZombieBeads() int {
	if a.beadsManager == nil {
		return 0
	}

	inProgressBeads, err := a.beadsManager.ListBeads(map[string]interface{}{
		"status": models.BeadStatusInProgress,
	})
	if err != nil {
		log.Printf("[Loom] resetZombieBeads: could not list in-progress beads: %v", err)
		return 0
	}

	// Build a set of known live agent IDs so we can detect beads assigned to
	// named agents that no longer exist or are permanently idle.
	liveAgentIDs := make(map[string]bool)
	if a.agentManager != nil {
		for _, ag := range a.agentManager.ListAgents() {
			if ag != nil {
				liveAgentIDs[ag.ID] = true
			}
		}
	}

	count := 0
	for _, b := range inProgressBeads {
		if b == nil || b.AssignedTo == "" {
			continue
		}
		isZombie := false
		if strings.HasPrefix(b.AssignedTo, "exec-") {
			// Ephemeral goroutine ID — never survives restart
			isZombie = true
		} else if !liveAgentIDs[b.AssignedTo] {
			// Named agent ID that is not registered in the current run
			isZombie = true
		}
		if !isZombie {
			continue
		}
		if err := a.beadsManager.UpdateBead(b.ID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": "",
		}); err != nil {
			log.Printf("[Loom] resetZombieBeads: could not reset bead %s: %v", b.ID, err)
			continue
		}
		log.Printf("[Loom] Recovered zombie bead %s [%s] (was held by stale assignee %s)",
			b.ID, b.Title, b.AssignedTo)
		count++
	}
	return count
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
