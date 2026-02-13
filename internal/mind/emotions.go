package mind

import (
	"time"
)

// EmotionDecayPerSecond — emotions drift toward zero over time.
const EmotionDecayPerSecond = 0.002

// ApplyEmotionDecay reduces emotion values toward 0. since = time since last update.
func ApplyEmotionDecay(e *Emotions, since time.Duration) *Emotions {
	if e == nil {
		return &Emotions{}
	}
	out := *e
	sec := since.Seconds()
	if sec < 0 {
		sec = 0
	}
	decay := 1.0 - EmotionDecayPerSecond*sec
	if decay < 0 {
		decay = 0
	}
	out.Anger = clamp01(out.Anger * decay)
	out.Joy = clamp01(out.Joy * decay)
	out.Fatigue = clamp01(out.Fatigue * decay)
	out.Engagement = clamp01(out.Engagement * decay)
	out.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return &out
}

// DecayEmotionsSinceLastUpdate parses UpdatedAt and applies decay until now.
func DecayEmotionsSinceLastUpdate(e *Emotions, now time.Time) *Emotions {
	if e == nil {
		return &Emotions{}
	}
	var since time.Duration
	if e.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, e.UpdatedAt); err == nil {
			since = now.Sub(t)
		}
	}
	return ApplyEmotionDecay(e, since)
}

// ApplyEvent applies a simple emotional event (e.g. negative -> anger up, trust down).
func ApplyEvent(e *Emotions, positive bool, intensity float64) *Emotions {
	if e == nil {
		e = &Emotions{}
	}
	out := *e
	intensity = clamp01(intensity)
	if positive {
		out.Joy = clamp01(out.Joy + intensity*0.3)
		out.Engagement = clamp01(out.Engagement + intensity*0.2)
	} else {
		out.Anger = clamp01(out.Anger + intensity*0.3)
		out.Fatigue = clamp01(out.Fatigue + intensity*0.1)
	}
	return &out
}

// EmotionalActivation returns a 0..1 value for "how activated" the agent is (affects DesireToSpeak).
func EmotionalActivation(e *Emotions) float64 {
	if e == nil {
		return 0
	}
	// anger and joy both increase tendency to speak; fatigue decreases
	act := (e.Anger + e.Joy) * 0.5 - e.Fatigue*0.3 + e.Engagement*0.4
	return clamp01(act)
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// ClampWorldviewDelta ensures no trait changes by more than maxDelta (e.g. 0.05).
func ClampWorldviewDelta(current, delta, maxDelta float64) float64 {
	d := delta
	if d > maxDelta {
		d = maxDelta
	}
	if d < -maxDelta {
		d = -maxDelta
	}
	return clamp01(current + d)
}

// ClampWorldviewRange clamps all worldview fields to [0,1].
func ClampWorldviewRange(w *Worldview) {
	if w == nil {
		return
	}
	w.TrustInPeople = clamp01(w.TrustInPeople)
	w.Cynicism = clamp01(w.Cynicism)
	w.Openness = clamp01(w.Openness)
	w.LoyaltyBias = clamp01(w.LoyaltyBias)
}
