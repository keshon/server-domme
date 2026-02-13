package mind

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"server-domme/internal/ai"
)

// WorldviewEvolutionPrompt asks LLM for small delta suggestions only. Biology/temperament never changes.
const WorldviewEvolutionPrompt = `You are an observer. Given the character's biology (fixed), current worldview, and guild context, suggest tiny adjustments as JSON only. Each value must be between -0.05 and 0.05 (deltas to add). Output only valid JSON with these keys: trust_in_people, cynicism, optimism, patience, skepticism, attachment_to_regulars, sensitivity_to_disrespect, need_for_recognition, tolerance_for_chaos, risk_taking, value_of_loyalty, importance_of_intellectual_depth. If no change for a key, use 0.`

const MaxWorldviewDelta = 0.05

// worldviewDelta is the LLM response format (deltas only).
type worldviewDelta struct {
	TrustInPeople                float64 `json:"trust_in_people"`
	Cynicism                     float64 `json:"cynicism"`
	Optimism                     float64 `json:"optimism"`
	Patience                     float64 `json:"patience"`
	Skepticism                   float64 `json:"skepticism"`
	AttachmentToRegulars         float64 `json:"attachment_to_regulars"`
	SensitivityToDisrespect      float64 `json:"sensitivity_to_disrespect"`
	NeedForRecognition           float64 `json:"need_for_recognition"`
	ToleranceForChaos            float64 `json:"tolerance_for_chaos"`
	RiskTaking                   float64 `json:"risk_taking"`
	ValueOfLoyalty               float64 `json:"value_of_loyalty"`
	ImportanceOfIntellectualDepth float64 `json:"importance_of_intellectual_depth"`
}

var worldviewDeltaRegex = regexp.MustCompile(`\{[^{}]*"trust_in_people"\s*:\s*[-\d.]+[^{}]*\}`)

// EvolveWorldview calls LLM for delta suggestions, applies with clamp, never changes biology/temperament.
func EvolveWorldview(provider ai.Provider, core *Core, g *GuildState, guildID string) error {
	bio := core.GetBiology()
	w := core.GetWorldview()
	mm := g.GetMediumMemory()

	content := fmt.Sprintf("Biology (fixed): temperament O=%.2f C=%.2f E=%.2f A=%.2f N=%.2f dominance=%.2f emotional_reactivity=%.2f\nCurrent worldview: trust_in_people=%.2f cynicism=%.2f optimism=%.2f patience=%.2f skepticism=%.2f attachment_to_regulars=%.2f sensitivity_to_disrespect=%.2f need_for_recognition=%.2f tolerance_for_chaos=%.2f risk_taking=%.2f value_of_loyalty=%.2f importance_of_intellectual_depth=%.2f\nGuild context:\n%s",
		bio.Temperament.Openness, bio.Temperament.Conscientiousness, bio.Temperament.Extraversion, bio.Temperament.Agreeableness, bio.Temperament.Neuroticism,
		bio.Dominance, bio.EmotionalReactivity,
		w.TrustInPeople, w.Cynicism, w.Optimism, w.Patience, w.Skepticism, w.AttachmentToRegulars, w.SensitivityToDisrespect, w.NeedForRecognition, w.ToleranceForChaos, w.RiskTaking, w.ValueOfLoyalty, w.ImportanceOfIntellectualDepth,
		string(mm))
	if len(content) > 2500 {
		content = content[:2500]
	}

	log.Printf("[MIND] evolution prompt guild=%s system_len=%d user_len=%d", guildID, len(WorldviewEvolutionPrompt), len(content))
	log.Printf("[MIND] evolution user_content: %s", truncateForLog(content, 400))

	messages := []ai.Message{
		{Role: "system", Content: WorldviewEvolutionPrompt},
		{Role: "user", Content: content},
	}
	out, err := provider.Generate(messages)
	if err != nil {
		return err
	}

	raw := strings.TrimSpace(out)
	if idx := worldviewDeltaRegex.FindStringIndex(raw); len(idx) > 0 {
		raw = raw[idx[0]:idx[1]]
	}
	// Try to parse full JSON (may span multiple lines)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var delta worldviewDelta
	if err := json.Unmarshal([]byte(raw), &delta); err != nil {
		return err
	}

	next := &Worldview{
		TrustInPeople:                ClampWorldviewDelta(w.TrustInPeople, delta.TrustInPeople, MaxWorldviewDelta),
		Cynicism:                     ClampWorldviewDelta(w.Cynicism, delta.Cynicism, MaxWorldviewDelta),
		Optimism:                     ClampWorldviewDelta(w.Optimism, delta.Optimism, MaxWorldviewDelta),
		Patience:                     ClampWorldviewDelta(w.Patience, delta.Patience, MaxWorldviewDelta),
		Skepticism:                   ClampWorldviewDelta(w.Skepticism, delta.Skepticism, MaxWorldviewDelta),
		AttachmentToRegulars:         ClampWorldviewDelta(w.AttachmentToRegulars, delta.AttachmentToRegulars, MaxWorldviewDelta),
		SensitivityToDisrespect:      ClampWorldviewDelta(w.SensitivityToDisrespect, delta.SensitivityToDisrespect, MaxWorldviewDelta),
		NeedForRecognition:           ClampWorldviewDelta(w.NeedForRecognition, delta.NeedForRecognition, MaxWorldviewDelta),
		ToleranceForChaos:            ClampWorldviewDelta(w.ToleranceForChaos, delta.ToleranceForChaos, MaxWorldviewDelta),
		RiskTaking:                   ClampWorldviewDelta(w.RiskTaking, delta.RiskTaking, MaxWorldviewDelta),
		ValueOfLoyalty:               ClampWorldviewDelta(w.ValueOfLoyalty, delta.ValueOfLoyalty, MaxWorldviewDelta),
		ImportanceOfIntellectualDepth: ClampWorldviewDelta(w.ImportanceOfIntellectualDepth, delta.ImportanceOfIntellectualDepth, MaxWorldviewDelta),
		UpdatedAt:                    time.Now().UTC().Format(time.RFC3339),
	}
	ClampWorldviewRange(next)
	log.Printf("[MIND] evolution applied guild=%s (deltas applied, worldview saved)", guildID)
	return core.SaveWorldview(next)
}
