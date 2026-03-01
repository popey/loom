package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jordanhubbard/loom/internal/persona"
	"github.com/jordanhubbard/loom/pkg/models"
)

// SkillLoader loads and injects skills into LLM prompts.
// Skill portability: invoke_skill loads target SKILL.md and injects it into the LLM prompt.
type SkillLoader struct {
	personaManager *persona.Manager
}

// NewSkillLoader creates a new skill loader
func NewSkillLoader(personaManager *persona.Manager) *SkillLoader {
	return &SkillLoader{
		personaManager: personaManager,
	}
}

// LoadSkill loads a skill from a persona's SKILL.md file.
// Returns the skill content that can be injected into an LLM prompt.
func (sl *SkillLoader) LoadSkill(personaName string) (string, error) {
	if personaName == "" {
		return "", fmt.Errorf("persona name is required")
	}

	// Load the persona (which includes SKILL.md)
	p, err := sl.personaManager.LoadPersona(personaName)
	if err != nil {
		return "", fmt.Errorf("failed to load persona %s: %w", personaName, err)
	}

	// Return the skill instructions (from SKILL.md body)
	if p.Instructions == "" {
		return "", fmt.Errorf("skill instructions empty for persona %s", personaName)
	}

	return p.Instructions, nil
}

// InvokeSkill loads a target skill and returns it formatted for injection into an LLM prompt.
// This enables skill portability: any agent can load any skill and use it.
func (sl *SkillLoader) InvokeSkill(targetPersonaName string) (string, error) {
	skillContent, err := sl.LoadSkill(targetPersonaName)
	if err != nil {
		return "", err
	}

	// Format the skill for injection into the prompt
	return sl.formatSkillForInjection(targetPersonaName, skillContent), nil
}

// formatSkillForInjection wraps the skill content with clear markers for the LLM
func (sl *SkillLoader) formatSkillForInjection(personaName, skillContent string) string {
	return fmt.Sprintf(
		"\n\n=== INJECTED SKILL: %s ===\n\n%s\n\n=== END INJECTED SKILL ===\n\n",
		personaName,
		skillContent,
	)
}

// GetAvailableSkills returns a list of all available skills (personas)
func (sl *SkillLoader) GetAvailableSkills() ([]string, error) {
	return sl.personaManager.ListPersonas()
}

// SkillInjectionContext represents a skill injection into a prompt
type SkillInjectionContext struct {
	TargetPersona string // The persona whose skill is being invoked
	SkillContent  string // The actual skill content
	InjectedAt    string // When it was injected (for audit trail)
}

// BuildPromptWithSkill builds an LLM prompt that includes an injected skill.
// This allows an agent to temporarily adopt another agent's skill.
func (sl *SkillLoader) BuildPromptWithSkill(basePrompt, targetPersonaName string) (string, *SkillInjectionContext, error) {
	skillContent, err := sl.InvokeSkill(targetPersonaName)
	if err != nil {
		return "", nil, err
	}

	ctx := &SkillInjectionContext{
		TargetPersona: targetPersonaName,
		SkillContent:  skillContent,
		InjectedAt:    "prompt-build",
	}

	// Inject the skill into the prompt
	finalPrompt := basePrompt + skillContent

	return finalPrompt, ctx, nil
}

// LoadSkillFromFile loads a skill directly from a SKILL.md file path.
// Useful for testing or loading skills from non-standard locations.
func LoadSkillFromFile(filePath string) (string, error) {
	if filePath == "" {
		return "", fmt.Errorf("file path is required")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read skill file: %w", err)
	}

	// Parse the SKILL.md format (YAML frontmatter + body)
	body, err := extractSkillBody(string(content))
	if err != nil {
		return "", err
	}

	return body, nil
}

// extractSkillBody extracts the body (non-frontmatter) from SKILL.md content
func extractSkillBody(content string) (string, error) {
	if !strings.HasPrefix(content, "---\n") {
		return "", fmt.Errorf("missing frontmatter: SKILL.md must start with '---'")
	}

	parts := strings.SplitN(content[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed frontmatter: missing closing '---'")
	}

	return strings.TrimSpace(parts[1]), nil
}

// SkillRegistry maintains a registry of available skills and their metadata
type SkillRegistry struct {
	skills map[string]*SkillMetadata
}

// SkillMetadata represents metadata about a skill
type SkillMetadata struct {
	Name          string   // e.g., "coder", "qa-engineer"
	Description   string   // What this skill does
	Specialties   []string // Areas of expertise
	Level         string   // beginner, intermediate, expert
	Prerequisites []string // Skills that should be learned first
}

// NewSkillRegistry creates a new skill registry
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]*SkillMetadata),
	}
}

// RegisterSkill registers a skill in the registry
func (sr *SkillRegistry) RegisterSkill(metadata *SkillMetadata) error {
	if metadata.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	sr.skills[metadata.Name] = metadata
	return nil
}

// GetSkill retrieves skill metadata
func (sr *SkillRegistry) GetSkill(name string) (*SkillMetadata, error) {
	if skill, ok := sr.skills[name]; ok {
		return skill, nil
	}
	return nil, fmt.Errorf("skill not found: %s", name)
}

// ListSkills returns all registered skills
func (sr *SkillRegistry) ListSkills() []*SkillMetadata {
	var skills []*SkillMetadata
	for _, skill := range sr.skills {
		skills = append(skills, skill)
	}
	return skills
}
