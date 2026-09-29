package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Intent names the closed set the router may dispatch. Anything else is
// refused: the model proposes, the code commits, and the code only knows
// these four.
const (
	IntentHelp       = "help.route"
	IntentSummarize  = "summarize"
	IntentKnowledge  = "knowledge.ask"
	IntentTaskAsk    = "task.request"
	IntentUnknown    = "unknown"
)

// Intent is one parsed request: what to do plus the args the executor reads.
type Intent struct {
	Name       string         `json:"intent"`
	Args       map[string]any `json:"args"`
	Confidence float64        `json:"confidence"`
	Raw        string         `json:"-"`
}

const routerSystem = `You route a Discord message to one bot capability. Answer JSON only, no prose.
Intents: "help.route" (what can you do, how do I use X), "summarize" (what happened, what did I miss, recap), "knowledge.ask" (rules, guides, lore, server info questions), "task.request" (give me a task, dare, assignment), "unknown" (anything else).
Fields: {"intent": "...", "args": {"query": "...", "limit": 50, "duration_min": 0}, "confidence": 0.0-1.0}.
For summarize set limit 10-100 from the text or 50. For task.request set duration_min when a duration is named else 0 and query to the raw wish. For knowledge.ask and help.route put the question in query. Confidence below 0.5 means unknown.`

// RouterPrompt builds the classification messages for text.
func RouterPrompt(text string) []Message {
	return []Message{
		{Role: "system", Content: routerSystem},
		{Role: "user", Content: text},
	}
}

// ParseIntent classifies text via LLM. Low confidence, bad JSON or unknown
// names all collapse to IntentUnknown with the error for logging.
func ParseIntent(ctx context.Context, c *Client, text string) Intent {
	low := 0.4
	raw, err := c.Complete(ctx, RouterPrompt(text), ptrFloat(float64(low)))
	if err != nil {
		return Intent{Name: IntentUnknown, Raw: "", Confidence: 0}
	}
	return DecodeIntent(raw)
}

// DecodeIntent parses router JSON without any I/O, so it is unit-testable.
func DecodeIntent(raw string) Intent {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)
	var in struct {
		Intent     string         `json:"intent"`
		Args       map[string]any `json:"args"`
		Confidence float64        `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(cleaned), &in); err != nil {
		return Intent{Name: IntentUnknown, Raw: raw}
	}
	switch in.Intent {
	case IntentHelp, IntentSummarize, IntentKnowledge, IntentTaskAsk, IntentUnknown:
	default:
		return Intent{Name: IntentUnknown, Raw: raw}
	}
	if in.Confidence < 0.5 && in.Intent != IntentUnknown {
		in.Intent = IntentUnknown
	}
	if in.Args == nil {
		in.Args = map[string]any{}
	}
	return Intent{Name: in.Intent, Args: in.Args, Confidence: in.Confidence, Raw: raw}
}

// StringArg reads a string arg safely for executors.
func (in Intent) StringArg(key string) string {
	v, ok := in.Args[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// IntArg reads a numeric arg safely for executors.
func (in Intent) IntArg(key string) int {
	v, ok := in.Args[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func ptrFloat(f float64) *float64 { return &f }

// KeywordFallback is the deterministic classifier when the LLM is disabled
// or failed: substring matching only, no semantics. It keeps the router
// usable offline and is what tests pin.
func KeywordFallback(text string) Intent {
	t := strings.ToLower(text)
	switch {
	case strings.Contains(t, "summar") || strings.Contains(t, "recap") || strings.Contains(t, "missed") || strings.Contains(t, "catch up") || strings.Contains(t, "catchup"):
		return Intent{Name: IntentSummarize, Args: map[string]any{"limit": 50}, Confidence: 0.6}
	case strings.Contains(t, "task") || strings.Contains(t, "dare") || strings.Contains(t, "assign"):
		return Intent{Name: IntentTaskAsk, Args: map[string]any{"query": text}, Confidence: 0.6}
	case strings.Contains(t, "rule") || strings.Contains(t, "guide") || strings.Contains(t, "lore") || strings.Contains(t, "what is") || strings.Contains(t, "what are") || strings.Contains(t, "how do") || strings.Contains(t, "?"):
		return Intent{Name: IntentKnowledge, Args: map[string]any{"query": text}, Confidence: 0.55}
	case strings.Contains(t, "help") || strings.Contains(t, "what can you") || strings.Contains(t, "how to") || strings.Contains(t, "command"):
		return Intent{Name: IntentHelp, Args: map[string]any{"query": text}, Confidence: 0.6}
	default:
		return Intent{Name: IntentUnknown, Confidence: 0}
	}
}

var _ = fmt.Sprint
