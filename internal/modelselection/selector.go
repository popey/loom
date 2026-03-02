package modelselection

import (
	"fmt"
	"strings"

	"github.com/jordanhubbard/loom/pkg/models"
)

// ModelTier represents the capability tier of an LLM model
type ModelTier string

const (
	TierLightweight ModelTier = "lightweight" // Fast, cheap, for trivial tasks
	TierMidTier     ModelTier = "mid-tier"    // Balanced capability and cost
	TierStrong      ModelTier = "strong"      // Strongest available, for complex decisions
)

// Selector maps bead priority and complexity to model tier and passes preference to TokenHub
type Selector struct {
	defaultModel string // Default model to use if no preference
}

// NewSelector creates a new model selector
func NewSelector(defaultModel string) *Selector {
	return &Selector{
		defaultModel: defaultModel,
	}
}

// SelectModelForBead selects the appropriate model tier for a bead based on priority and complexity
func (s *Selector) SelectModelForBead(bead *models.Bead) (ModelTier, string, error) {
	if bead == nil {
		return "", "", fmt.Errorf("bead is required")
	}

	// Map priority to model tier
	tier := s.priorityToTier(bead.Priority)

	// Get the model name for this tier
	modelName := s.tierToModel(tier)

	return tier, modelName, nil
}

// priorityToTier maps bead priority to model tier
func (s *Selector) priorityToTier(priority models.BeadPriority) ModelTier {
	switch priority {
	case models.BeadPriorityP0:
		return TierStrong
	case models.BeadPriorityP1:
		return TierStrong
	case models.BeadPriorityP2:
		return TierMidTier
	case models.BeadPriorityP3:
		return TierLightweight
	default:
		return TierMidTier
	}
}

// tierToModel maps model tier to actual model name
func (s *Selector) tierToModel(tier ModelTier) string {
	switch tier {
	case TierLightweight:
		return "gpt-4o-mini" // Fast, cheap
	case TierMidTier:
		return "gpt-4o" // Balanced
	case TierStrong:
		return "gpt-4-turbo" // Strongest
	default:
		return s.defaultModel
	}
}

// SelectModelForTask selects a model based on task type and complexity
func (s *Selector) SelectModelForTask(taskType string, complexity string) (ModelTier, string, error) {
	var tier ModelTier

	// Map task type and complexity to tier
	switch strings.ToLower(taskType) {
	case "trivial", "rename", "format", "comment":
		tier = TierLightweight
	case "standard", "implement", "test", "refactor":
		tier = TierMidTier
	case "complex", "architecture", "design", "decision":
		tier = TierStrong
	default:
		tier = TierMidTier
	}

	// Adjust based on complexity if provided
	if strings.ToLower(complexity) == "high" {
		if tier == TierLightweight {
			tier = TierMidTier
		} else if tier == TierMidTier {
			tier = TierStrong
		}
	}

	modelName := s.tierToModel(tier)
	return tier, modelName, nil
}

// BuildTokenHubPreference builds a preference string to pass to TokenHub
func (s *Selector) BuildTokenHubPreference(tier ModelTier) string {
	switch tier {
	case TierLightweight:
		return "model_preference=lightweight,cost_priority=true"
	case TierMidTier:
		return "model_preference=balanced,cost_priority=false"
	case TierStrong:
		return "model_preference=strongest,cost_priority=false"
	default:
		return ""
	}
}

// ModelPreference represents a preference to pass to TokenHub
type ModelPreference struct {
	Tier            ModelTier // The tier of model to use
	Model           string    // Specific model name
	CostPriority    bool      // Whether to prioritize cost over capability
	LatencyPriority bool      // Whether to prioritize latency
	QualityPriority bool      // Whether to prioritize quality
}

// BuildPreference builds a ModelPreference for a bead
func (s *Selector) BuildPreference(bead *models.Bead) (*ModelPreference, error) {
	tier, model, err := s.SelectModelForBead(bead)
	if err != nil {
		return nil, err
	}

	pref := &ModelPreference{
		Tier:  tier,
		Model: model,
	}

	// Set priorities based on tier
	switch tier {
	case TierLightweight:
		pref.CostPriority = true
		pref.LatencyPriority = true
	case TierMidTier:
		pref.QualityPriority = true
	case TierStrong:
		pref.QualityPriority = true
	}

	return pref, nil
}
