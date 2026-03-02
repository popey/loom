package taskexecutor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/persona"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/pkg/models"
)

type CEODecision struct {
	Action    string
	Rationale string
}

func MakeCEODecision(ctx context.Context, beadID string, bead *models.Bead, reason string, personaMgr *persona.Manager, providerRegistry *provider.Registry) (*CEODecision, error) {
	if personaMgr == nil {
		return nil, fmt.Errorf("persona manager not available")
	}

	ceoPers, err := personaMgr.LoadPersona("ceo")
	if err != nil {
		return nil, fmt.Errorf("failed to load CEO persona: %w", err)
	}

	beadContext := fmt.Sprintf("Bead ID: %s\nTitle: %s\nDescription: %s\nStatus: %s\nPriority: %d\nAssigned To: %s\nCreated At: %s\nEscalation Reason: %s\n", bead.ID, bead.Title, bead.Description, bead.Status, bead.Priority, bead.AssignedTo, bead.CreatedAt.Format(time.RFC3339), reason)

	prompt := fmt.Sprintf("You are %s. %s\n\nA bead (work item) requires your decision:\n\n%s\n\nBased on the escalation reason and the bead's context, decide what to do:\n- approve: Accept the bead as-is and close it\n- reassign: Reassign to a different agent or team\n- cull: Cancel/reject the bead as not worth pursuing\n- defer: Defer the decision and return to the original agent for more information\n\nRespond with ONLY:\nACTION: <action>\nRATIONALE: <your reasoning>\n\nBe direct and concise. Focus on outcomes and impact.", ceoPers.Name, ceoPers.Instructions, beadContext)

	if providerRegistry == nil {
		return nil, fmt.Errorf("provider registry not available")
	}

	providers := providerRegistry.ListActive()
	if len(providers) == 0 {
		return nil, fmt.Errorf("no active providers available")
	}

	proto := providers[0].Protocol

	req := &provider.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []provider.ChatMessage{
			{
				Role:    "system",
				Content: "You are a CEO making strategic decisions about work items. Be direct, decisive, and focused on outcomes.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.7,
		MaxTokens:   500,
	}

	resp, err := proto.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	responseText := resp.Choices[0].Message.Content

	decision := &CEODecision{}
	lines := strings.Split(responseText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ACTION:") {
			action := strings.TrimPrefix(line, "ACTION:")
			decision.Action = strings.ToLower(strings.TrimSpace(action))
		} else if strings.HasPrefix(line, "RATIONALE:") {
			rationale := strings.TrimPrefix(line, "RATIONALE:")
			decision.Rationale = strings.TrimSpace(rationale)
		}
	}

	validActions := map[string]bool{
		"approve":  true,
		"reassign": true,
		"cull":     true,
		"defer":    true,
	}
	if !validActions[decision.Action] {
		log.Printf("[CEO] Invalid action from LLM: %s, defaulting to defer", decision.Action)
		decision.Action = "defer"
	}

	if decision.Rationale == "" {
		decision.Rationale = "CEO decision made"
	}

	log.Printf("[CEO] Decision for bead %s: %s - %s", beadID, decision.Action, decision.Rationale)

	return decision, nil
}
