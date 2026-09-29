package task

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/keshon/server-domme/internal/llm"
)

// RequestSpec is the LLM's structured reading of a free-text task wish.
// The model proposes, the code commits: every field is advisory, the task
// list stays the source of truth, and an empty spec means "pick at random".
type RequestSpec struct {
	DurationMin int      `json:"duration_min"`
	Keywords    []string `json:"keywords"`
	Intensity   string   `json:"intensity"`
}

const requestSystem = `Read a task wish and answer JSON only, no prose.
Fields: {"duration_min": minutes named or 0, "keywords": ["1-6 lowercase content words"], "intensity": "gentle|standard|intense"}.
Rules: duration_min is a number or 0 when none is named. Keywords are content words from the wish (easy, quick, humiliating, public...), never names. Intensity defaults to standard. Confidence is not needed.`

// ParseRequestSpec turns free text into a RequestSpec via LLM. Any failure
// (disabled, unconfigured, bad JSON) returns an empty spec so the caller
// falls back to a random pick: NL refines selection, never blocks it.
func ParseRequestSpec(ctx context.Context, client llm.Provider, text string) RequestSpec {
	if client == nil || strings.TrimSpace(text) == "" {
		return RequestSpec{}
	}
	raw, err := client.Complete(ctx, []llm.Message{
		{Role: "system", Content: requestSystem},
		{Role: "user", Content: text},
	}, ptrTemp(0.3))
	if err != nil {
		return RequestSpec{}
	}
	return DecodeRequestSpec(raw)
}

// DecodeRequestSpec parses router JSON without I/O, for tests.
func DecodeRequestSpec(raw string) RequestSpec {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	var spec RequestSpec
	if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &spec); err != nil {
		return RequestSpec{}
	}
	if spec.DurationMin < 0 {
		spec.DurationMin = 0
	}
	if spec.DurationMin > 180 {
		spec.DurationMin = 180
	}
	kept := make([]string, 0, len(spec.Keywords))
	for _, k := range spec.Keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" || len(k) > 30 {
			continue
		}
		kept = append(kept, k)
		if len(kept) == 6 {
			break
		}
	}
	spec.Keywords = kept
	switch spec.Intensity {
	case "gentle", "standard", "intense":
	default:
		spec.Intensity = "standard"
	}
	return spec
}

// ScoreTasks ranks role-filtered tasks against a spec. Duration proximity
// counts when named, keyword overlap always counts. Empty spec scores zero
// everywhere so the caller picks at random.
func ScoreTasks(tasks []Task, spec RequestSpec) []int {
	scores := make([]int, len(tasks))
	if len(spec.Keywords) == 0 && spec.DurationMin == 0 {
		return scores
	}
	for i, t := range tasks {
		desc := strings.ToLower(t.Description)
		for _, k := range spec.Keywords {
			if k != "" && strings.Contains(desc, k) {
				scores[i] += 2
			}
		}
		if spec.DurationMin > 0 && t.DurationMin > 0 {
			diff := t.DurationMin - spec.DurationMin
			if diff < 0 {
				diff = -diff
			}
			if diff <= 5 {
				scores[i] += 3
			} else if diff <= 15 {
				scores[i] += 1
			}
		}
	}
	return scores
}

// PickByScore returns the index of the best-scoring task, or -1 when every
// score is zero (caller falls back to random).
func PickByScore(scores []int) int {
	best, bestIdx := 0, -1
	for i, s := range scores {
		if s > best {
			best, bestIdx = s, i
		}
	}
	return bestIdx
}

const rephraseSystem = `Rephrase this task in the same dominant, teasing voice, same meaning and roughly the same length. One line out, no quotes, no preamble. Never add new acts, locations, or people.`

// RephraseTask rephrases one picked task description. Failure returns the
// original: variant polishes, never blocks.
func RephraseTask(ctx context.Context, client llm.Provider, description string) string {
	if client == nil || strings.TrimSpace(description) == "" {
		return description
	}
	raw, err := client.Complete(ctx, []llm.Message{
		{Role: "system", Content: rephraseSystem},
		{Role: "user", Content: description},
	}, ptrTemp(0.7))
	if err != nil {
		return description
	}
	out := llm.Sanitize(strings.TrimSpace(raw), 500)
	if out == "" {
		return description
	}
	// One line: a variant that sprawls into paragraphs is a new task.
	if idx := strings.Index(out, "\n"); idx >= 0 {
		out = strings.TrimSpace(out[:idx])
	}
	if out == "" {
		return description
	}
	return out
}

func ptrTemp(f float64) *float64 { return &f }
