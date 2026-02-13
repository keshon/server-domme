package mind

import (
	"log"
	"strings"

	"server-domme/internal/ai"
)

// LogLLMCall logs the combined prompt and params before sending to AI. Call immediately before provider.Generate.
func LogLLMCall(action string, messages []ai.Message, params map[string]string) {
	var parts []string
	for k, v := range params {
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	paramStr := strings.Join(parts, " ")
	log.Printf("[MIND] action=%s %s messages=%d", action, paramStr, len(messages))
	if len(messages) == 0 {
		return
	}
	sysLen := len(messages[0].Content)
	preview := messages[0].Content
	if len(preview) > 500 {
		preview = preview[:500] + "..."
	}
	log.Printf("[MIND] system_len=%d system_preview: %s", sysLen, preview)
	for i := 1; i < len(messages); i++ {
		m := messages[i]
		p := m.Content
		if len(p) > 200 {
			p = p[:200] + "..."
		}
		log.Printf("[MIND] msg[%d] role=%s len=%d: %s", i, m.Role, len(m.Content), p)
	}
}
