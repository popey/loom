package loom

import (
	"fmt"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/decision"
	"github.com/jordanhubbard/loom/internal/eventbus"
	"github.com/jordanhubbard/loom/pkg/models"
)

func (a *Loom) GetDecisionManager() *decision.Manager {
	return a.decisionManager
}
func (a *Loom) MakeDecision(decisionID, deciderID, decisionText, rationale string) error {
	// Verify decider exists (could be agent or user)
	// For users, we'll allow any decider ID starting with "user-"
	if !strings.HasPrefix(deciderID, "user-") {
		if _, err := a.agentManager.GetAgent(deciderID); err != nil {
			return fmt.Errorf("decider not found: %w", err)
		}
	}

	// Make decision
	if err := a.decisionManager.MakeDecision(decisionID, deciderID, decisionText, rationale); err != nil {
		return fmt.Errorf("failed to make decision: %w", err)
	}

	// Unblock dependent beads
	if err := a.UnblockDependents(decisionID); err != nil {
		return fmt.Errorf("failed to unblock dependents: %w", err)
	}

	if a.eventBus != nil {
		if d, err := a.decisionManager.GetDecision(decisionID); err == nil && d != nil {
			_ = a.eventBus.Publish(&eventbus.Event{
				Type:      eventbus.EventTypeDecisionResolved,
				Source:    "decision-manager",
				ProjectID: d.ProjectID,
				Data: map[string]interface{}{
					"decision_id": decisionID,
					"decision":    decisionText,
					"decider_id":  deciderID,
				},
			})
		}
	}

	_ = a.applyCEODecisionToParent(decisionID)

	return nil
}
func (a *Loom) applyCEODecisionToParent(decisionID string) error {
	d, err := a.decisionManager.GetDecision(decisionID)
	if err != nil || d == nil || d.Context == nil {
		return nil
	}
	if d.Context["escalated_to"] != "ceo" {
		return nil
	}
	parentID := d.Parent
	if parentID == "" {
		return nil
	}

	decision := strings.ToLower(strings.TrimSpace(d.Decision))
	switch decision {
	case "approve":
		_, _ = a.UpdateBead(parentID, map[string]interface{}{"status": models.BeadStatusClosed})
	case "deny":
		// Reassign to default triage agent instead of leaving unassigned
		denyAssignee := ""
		if parentBead, err := a.beadsManager.GetBead(parentID); err == nil {
			denyAssignee = a.findDefaultAssignee(parentBead.ProjectID)
		}
		_, _ = a.UpdateBead(parentID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": denyAssignee,
			"context": map[string]string{
				"ceo_denied_at":      time.Now().UTC().Format(time.RFC3339),
				"ceo_comment":        d.Rationale,
				"reassigned_to_role": "default-triage",
			},
		})
	case "needs_more_info":
		returnedTo := d.Context["returned_to"]
		_, _ = a.UpdateBead(parentID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": returnedTo,
			"context": map[string]string{
				"redispatch_requested":   "true",
				"ceo_needs_more_info_at": time.Now().UTC().Format(time.RFC3339),
				"ceo_comment":            d.Rationale,
			},
		})
	}

	return nil
}
func (a *Loom) GetPendingDecisions() ([]string, error) {
	if a.consensusManager == nil {
		return nil, fmt.Errorf("consensus manager not available")
	}
	// TODO: Implement pending decision retrieval
	return []string{}, nil
}
