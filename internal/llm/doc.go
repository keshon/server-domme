// Package llm is the stateless tool backend: one OpenAI-compatible call per
// user request, no memory, no autonomy, no persona.
//
// The mention router calls ParseIntent, slash helpers call Complete. Both
// fail closed: disabled, unconfigured, timeout or bad JSON means the caller
// falls back to deterministic behavior and tells the user how.
package llm

import (
	"github.com/keshon/server-domme/internal/config"
)

// Ready reports whether an LLM call may be attempted.
func Ready(cfg *config.Config) bool {
	if cfg == nil || !cfg.LLMEnabled {
		return false
	}
	return cfg.LLMBaseURL != "" && cfg.LLMModel != ""
}
