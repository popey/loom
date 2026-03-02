package loom

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/actions"
	internalmodels "github.com/jordanhubbard/loom/internal/models"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/pkg/models"
)

// ReplResult holds the result of a CEO REPL query.
type ReplResult struct {
	BeadID       string `json:"bead_id"`
	ProviderID   string `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	Model        string `json:"model"`
	Response     string `json:"response"`
	TokensUsed   int    `json:"tokens_used"`
	LatencyMs    int64  `json:"latency_ms"`
}

// RunReplQuery sends a high-priority query to the best provider.
func (a *Loom) RunReplQuery(ctx context.Context, message string) (*ReplResult, error) {
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("message is required")
	}
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}

	personaHint, cleanMessage := extractPersonaFromMessage(message)

	beadTitle := "CEO Query"
	if personaHint != "" {
		beadTitle = fmt.Sprintf("CEO Query for %s", personaHint)
	}
	if len(cleanMessage) < 80 {
		beadTitle = fmt.Sprintf("CEO: %s", cleanMessage)
	}

	bead, err := a.beadsManager.CreateBead(beadTitle, cleanMessage, models.BeadPriorityP0, "task", a.config.GetSelfProjectID())
	if err != nil {
		log.Printf("Warning: Failed to create CEO query bead: %v", err)
	}

	var beadID string
	if bead != nil {
		beadID = bead.ID
		if personaHint != "" {
			_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
				"description": fmt.Sprintf("Persona: %s\n\n%s", personaHint, cleanMessage),
			})
		}
	}

	providerRecord, err := a.selectBestProviderForRepl()
	if err != nil {
		return nil, err
	}

	systemPrompt := a.buildLoomPersonaPrompt()
	regProvider, err := a.providerRegistry.Get(providerRecord.ID)
	if err != nil {
		return nil, fmt.Errorf("provider %s not found in registry: %w", providerRecord.ID, err)
	}
	if regProvider.Protocol == nil {
		return nil, fmt.Errorf("provider %s has no protocol configured", providerRecord.ID)
	}

	model := providerRecord.SelectedModel
	if model == "" {
		model = providerRecord.Model
	}
	if model == "" {
		model = providerRecord.ConfiguredModel
	}

	req := &provider.ChatCompletionRequest{
		Model: model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: cleanMessage},
		},
		Temperature: 0.2,
		MaxTokens:   1200,
	}

	queryStart := time.Now()
	resp, err := regProvider.Protocol.CreateChatCompletion(ctx, req)
	latencyMs := time.Since(queryStart).Milliseconds()
	if err != nil {
		return nil, err
	}

	responseText := ""
	if len(resp.Choices) > 0 {
		responseText = resp.Choices[0].Message.Content
	}
	tokensUsed := resp.Usage.TotalTokens
	responseModel := resp.Model

	var actionResults []actions.Result
	if a.actionRouter != nil {
		actx := actions.ActionContext{
			AgentID:   "ceo",
			BeadID:    beadID,
			ProjectID: a.config.GetSelfProjectID(),
		}
		env, parseErr := actions.DecodeLenient([]byte(responseText))
		if parseErr != nil {
			actionResult := a.actionRouter.AutoFileParseFailure(ctx, actx, parseErr, responseText)
			actionResults = []actions.Result{actionResult}
		} else {
			actionResults, _ = a.actionRouter.Execute(ctx, env, actx)
		}
	}

	if beadID != "" {
		actionsJSON, _ := json.Marshal(actionResults)
		_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
			"context": map[string]string{
				"source":      "ceo-repl",
				"created_by":  "ceo",
				"response":    responseText,
				"actions":     string(actionsJSON),
				"provider_id": providerRecord.ID,
				"model":       responseModel,
				"tokens_used": fmt.Sprintf("%d", tokensUsed),
			},
			"status": models.BeadStatusClosed,
		})
	}

	if responseModel == "" {
		responseModel = providerRecord.SelectedModel
	}
	if responseModel == "" {
		responseModel = providerRecord.Model
	}
	return &ReplResult{
		BeadID:       beadID,
		ProviderID:   providerRecord.ID,
		ProviderName: providerRecord.Name,
		Model:        responseModel,
		Response:     responseText,
		TokensUsed:   tokensUsed,
		LatencyMs:    latencyMs,
	}, nil
}

// extractPersonaFromMessage extracts persona hint from "persona: message" format
// Returns (personaHint, cleanMessage)
func extractPersonaFromMessage(message string) (string, string) {
	message = strings.TrimSpace(message)

	// Check for "persona: rest of message" format
	if idx := strings.Index(message, ":"); idx > 0 && idx < 50 {
		potentialPersona := strings.TrimSpace(message[:idx])
		// Check if it looks like a persona (single word or hyphenated, lowercase)
		if isLikelyPersona(potentialPersona) {
			restOfMessage := strings.TrimSpace(message[idx+1:])
			return potentialPersona, restOfMessage
		}
	}

	return "", message
}

func isLikelyPersona(s string) bool {
	s = strings.ToLower(s)
	// Must be 3-40 characters, contain only letters, hyphens, and spaces
	if len(s) < 3 || len(s) > 40 {
		return false
	}
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || ch == '-' || ch == ' ') {
			return false
		}
	}
	// Can't start or end with hyphen/space
	if s[0] == '-' || s[0] == ' ' || s[len(s)-1] == '-' || s[len(s)-1] == ' ' {
		return false
	}
	return true
}

func (a *Loom) selectBestProviderForRepl() (*internalmodels.Provider, error) {
	providers, err := a.database.ListProviders()
	if err != nil {
		return nil, err
	}

	// With TokenHub as the sole provider, just return the first healthy one.
	for _, p := range providers {
		if p == nil {
			continue
		}
		if p.Status == "healthy" || p.Status == "active" {
			return p, nil
		}
	}

	return nil, fmt.Errorf("no healthy providers available")
}

func (a *Loom) buildLoomPersonaPrompt() string {
	persona, err := a.personaManager.LoadPersona("loom")
	if err != nil {
		return fmt.Sprintf("You are Loom, the orchestration system. Respond to the CEO with clear guidance and actionable next steps.\n\n%s", actions.ActionPrompt)
	}

	focus := strings.Join(persona.FocusAreas, ", ")
	standards := strings.Join(persona.Standards, "; ")

	return fmt.Sprintf(
		"You are Loom, the orchestration system. Treat this as a high-priority CEO request.\n\nMission: %s\nCharacter: %s\nTone: %s\nFocus Areas: %s\nDecision Making: %s\nStandards: %s\n\n%s",
		strings.TrimSpace(persona.Mission),
		strings.TrimSpace(persona.Character),
		strings.TrimSpace(persona.Tone),
		strings.TrimSpace(focus),
		strings.TrimSpace(persona.DecisionMaking),
		strings.TrimSpace(standards),
		actions.ActionPrompt,
	)
}

// ListModelCatalog returns the recommended model catalog.
func (a *Loom) ListModelCatalog() []internalmodels.ModelSpec {
	if a.modelCatalog == nil {
		return nil
	}
	return a.modelCatalog.List()
}
