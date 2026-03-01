package loom

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/taskexecutor"
	"github.com/jordanhubbard/loom/pkg/models"
)

func (a *Loom) EscalateBeadToCEO(beadID, reason, returnedTo string) (*models.DecisionBead, error) {
	b, err := a.beadsManager.GetBead(beadID)
	if err != nil {
		return nil, fmt.Errorf("bead not found: %w", err)
	}
	if returnedTo == "" {
		returnedTo = b.AssignedTo
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ceoDec, err := taskexecutor.MakeCEODecision(ctx, beadID, b, reason, a.personaManager, a.providerRegistry)
	if err != nil {
		log.Printf("[CEO] LLM call failed: %v, falling back to defer", err)
		ceoDec = &taskexecutor.CEODecision{
			Action:    "defer",
			Rationale: fmt.Sprintf("LLM call failed: %v", err),
		}
	}

	var choices []string
	var decisionText string
	switch ceoDec.Action {
	case "approve":
		choices = []string{"approve"}
		decisionText = "approve"
	case "reassign":
		choices = []string{"reassign"}
		decisionText = "reassign"
	case "cull":
		choices = []string{"cull"}
		decisionText = "cull"
	default:
		choices = []string{"defer"}
		decisionText = "defer"
	}

	question := fmt.Sprintf("CEO decision required for bead %s (%s).\n\nReason: %s\n\nCEO Decision: %s\n\nRationale: %s", b.ID, b.Title, reason, ceoDec.Action, ceoDec.Rationale)
	decision, err := a.decisionManager.CreateDecision(question, beadID, "system", choices, "", models.BeadPriorityP0, b.ProjectID)
	if err != nil {
		return nil, err
	}

	if decision.Context == nil {
		decision.Context = make(map[string]string)
	}
	decision.Context["escalated_to"] = "ceo"
	decision.Context["returned_to"] = returnedTo
	decision.Context["escalation_reason"] = reason
	decision.Context["ceo_decision"] = ceoDec.Action
	decision.Context["ceo_rationale"] = ceoDec.Rationale

	decision.Decision = decisionText
	decision.Rationale = ceoDec.Rationale

	_, _ = a.UpdateBead(beadID, map[string]interface{}{
		"priority": models.BeadPriorityP0,
		"context": map[string]string{
			"escalated_to_ceo_at":          time.Now().UTC().Format(time.RFC3339),
			"escalated_to_ceo_reason":      reason,
			"escalated_to_ceo_decision_id": decision.ID,
			"ceo_decision":                 ceoDec.Action,
			"ceo_rationale":                ceoDec.Rationale,
		},
	})

	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeDecisionCreated,
			Source:    "ceo-escalation",
			ProjectID: b.ProjectID,
			Data: map[string]interface{}{
				"decision_id":   decision.ID,
				"bead_id":       beadID,
				"reason":        reason,
				"ceo_decision":  ceoDec.Action,
				"ceo_rationale": ceoDec.Rationale,
			},
		})
	}

	if a.commentsManager != nil {
		commentText := fmt.Sprintf("**CEO Decision**: %s\n\n**Rationale**: %s", strings.ToUpper(ceoDec.Action), ceoDec.Rationale)
		_ = a.commentsManager.AddComment(decision.ID, "system", commentText)
	}

	return decision, nil
}
