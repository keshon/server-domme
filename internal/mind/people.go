package mind

import (
	"strings"
	"time"
)

// PersonUpdateKind classifies a message for relation updates (no LLM — heuristic).
type PersonUpdateKind int

const (
	PersonUpdateNeutral PersonUpdateKind = iota
	PersonUpdatePositive
	PersonUpdateNegative
	PersonUpdateAggressive
)

// ClassifyMessageForPerson returns update kind from content heuristic (caps, length, punctuation).
func ClassifyMessageForPerson(content string) PersonUpdateKind {
	content = strings.TrimSpace(content)
	if content == "" {
		return PersonUpdateNeutral
	}
	upper, total := 0, 0
	for _, r := range content {
		total++
		if r >= 'A' && r <= 'Z' {
			upper++
		}
	}
	if total > 0 && upper*100/total > 30 && total < 100 {
		return PersonUpdateAggressive
	}
	if strings.HasSuffix(content, "!") && upper > 2 {
		return PersonUpdateAggressive
	}
	// Positive: contains thanks, please, short polite
	lower := strings.ToLower(content)
	if strings.Contains(lower, "thank") || strings.Contains(lower, "please") || strings.Contains(lower, "🙏") {
		return PersonUpdatePositive
	}
	if strings.Contains(lower, "idiot") || strings.Contains(lower, "stupid") || strings.Contains(lower, "shut up") {
		return PersonUpdateNegative
	}
	return PersonUpdateNeutral
}

// ApplyPersonUpdate updates Person relation values. Clamp to [0,1]. Delta small (e.g. 0.05).
func ApplyPersonUpdate(p *Person, kind PersonUpdateKind, delta float64) *Person {
	if p == nil {
		p = &Person{}
	}
	if delta <= 0 || delta > 0.2 {
		delta = 0.08
	}
	out := *p
	out.UserID = p.UserID
	switch kind {
	case PersonUpdatePositive:
		out.Affinity = clamp01(out.Affinity + delta)
		out.Trust = clamp01(out.Trust + delta*0.5)
		out.Irritation = clamp01(out.Irritation - delta*0.5)
	case PersonUpdateNegative:
		out.Irritation = clamp01(out.Irritation + delta)
		out.Trust = clamp01(out.Trust - delta*0.5)
	case PersonUpdateAggressive:
		out.Irritation = clamp01(out.Irritation + delta*1.2)
		out.Trust = clamp01(out.Trust - delta)
	default:
		// neutral: tiny engagement
	}
	out.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return &out
}

// BumpEmotionFromPerson: high irritation → anger boost; high affinity → warmth (joy) boost. Affects behavioral tone.
func BumpEmotionFromPerson(e *Emotions, affinity, irritation float64) *Emotions {
	if e == nil {
		e = &Emotions{}
	}
	out := *e
	if irritation > 0.6 {
		out.Anger = clamp01(out.Anger + 0.1)
	} else if irritation > 0.5 {
		out.Anger = clamp01(out.Anger + 0.05)
	}
	if affinity > 0.7 {
		out.Joy = clamp01(out.Joy + 0.1)
		out.Engagement = clamp01(out.Engagement + 0.05)
	}
	out.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return &out
}
