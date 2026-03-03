package loom

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

// runModelCheck probes all active providers and validates model tier.
// Returns error only when on_violation=="block".
func (a *Loom) runModelCheck(ctx context.Context) error {
	cfg := a.config.Agents.ModelCheck
	if !cfg.Enabled {
		return nil
	}

	// Wait up to 10s for at least one provider to become active
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(a.providerRegistry.ListActive()) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	providers := a.providerRegistry.ListActive()
	if len(providers) == 0 {
		log.Printf("[ModelCheck] No active providers to check")
		return nil
	}

	var violations []string
	for _, rp := range providers {
		model := rp.Config.SelectedModel
		if model == "" {
			model = rp.Config.Model
		}
		verdict, reason := classifyModel(model, cfg.Allowlist, cfg.DenylistPatterns, cfg.MinTier)
		if verdict == "blocked" {
			msg := fmt.Sprintf("provider %s model %q: %s", rp.Config.ID, model, reason)
			violations = append(violations, msg)
			log.Printf("[ModelCheck] VIOLATION: %s", msg)
		} else {
			log.Printf("[ModelCheck] provider %s model %q: OK", rp.Config.ID, model)
		}
	}

	if len(violations) == 0 {
		log.Printf("[ModelCheck] All %d provider(s) passed model tier check", len(providers))
		return nil
	}
	summary := strings.Join(violations, "; ")
	if cfg.OnViolation == "block" {
		return fmt.Errorf("startup blocked by model check: %s", summary)
	}
	log.Printf("[ModelCheck] WARNING: %s", summary)
	return nil
}

func classifyModel(modelID string, allowlist, denylist []string, minTier string) (string, string) {
	lower := strings.ToLower(modelID)
	for _, a := range allowlist {
		if strings.EqualFold(modelID, a) {
			return "ok", "explicitly allowlisted"
		}
	}
	for _, pattern := range denylist {
		if matched, _ := filepath.Match(strings.ToLower(pattern), lower); matched {
			return "blocked", fmt.Sprintf("matches denylist pattern %q", pattern)
		}
	}
	if minTier == "sonnet" && isBelowSonnet(lower) {
		return "blocked", "below minimum sonnet tier"
	}
	return "ok", "passes tier check"
}

func isBelowSonnet(lower string) bool {
	for _, marker := range []string{"1b", "3b", "7b", "8b"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, weak := range []string{"claude-haiku", "gemini-flash-lite", "flash-thinking-exp",
		"phi-", "mistral-7b", "llama-3-8b", "llama-3.1-8b", "minimax"} {
		if strings.Contains(lower, weak) {
			return true
		}
	}
	return false
}
