package confess

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/keshon/server-domme/internal/llm"
)

// Verdict is the guard's answer: block and why. The message itself is never
// echoed back, logged, or stored.
type Verdict struct {
	Risky  bool
	Reason string
}

const guardSystem = `You guard an anonymous confession channel. Answer JSON only, no prose.
Fields: {"risky": true|false, "reason": "one short line when risky, empty otherwise"}.
Mark risky=true only for: credible threats of violence, sexual content involving minors, doxxing (full names with addresses, phone numbers, outing someone's identity), or spam/gibberish with no readable content.
Everything else is risky=false: embarrassment, kink, insults, profanity, and dark humor stay. When in doubt, allow. Never include the message in reason.`

// CheckConfession runs the optional pre-post guard. Any failure (disabled,
// unconfigured, timeout, bad JSON) returns not-risky so confessions keep
// flowing: the guard refines, never blocks on backend trouble. Callers must
// not log the message alongside the verdict.
func CheckConfession(ctx context.Context, client llm.Provider, message string) Verdict {
	if client == nil || strings.TrimSpace(message) == "" {
		return Verdict{}
	}
	text := message
	if len(text) > 2000 {
		text = text[:2000]
	}
	raw, err := client.Complete(ctx, []llm.Message{
		{Role: "system", Content: guardSystem},
		{Role: "user", Content: text},
	}, ptrTemp(0.2))
	if err != nil {
		return Verdict{}
	}
	return DecodeVerdict(raw)
}

// DecodeVerdict parses guard JSON without I/O, for tests.
func DecodeVerdict(raw string) Verdict {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	var in struct {
		Risky  bool   `json:"risky"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &in); err != nil {
		return Verdict{}
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) > 200 {
		in.Reason = in.Reason[:200]
	}
	if !in.Risky {
		in.Reason = ""
	}
	return Verdict{Risky: in.Risky, Reason: llm.Sanitize(in.Reason, 200)}
}

func ptrTemp(f float64) *float64 { return &f }
