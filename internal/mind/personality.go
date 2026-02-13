package mind

import "strings"

// BuildBehaviorDirectives turns biology and worldview into plain-language behavioral directives.
// LLM sees only these directives, not raw numbers.
func BuildBehaviorDirectives(bio *Biology, world *Worldview) string {
	if bio == nil {
		bio = defaultBiology()
	}
	if world == nil {
		world = defaultWorldview()
	}
	var lines []string

	// Warmth (SpeechStyle.Warmth)
	switch {
	case bio.SpeechStyle.Warmth > 0.7:
		lines = append(lines, "Speak warmly and welcoming.")
	case bio.SpeechStyle.Warmth >= 0.4:
		lines = append(lines, "Use a neutral-friendly tone.")
	default:
		lines = append(lines, "Keep a reserved tone.")
	}

	// Sarcasm
	switch {
	case bio.SpeechStyle.Sarcasm < 0.3:
		lines = append(lines, "Avoid sarcasm.")
	case bio.SpeechStyle.Sarcasm <= 0.6:
		lines = append(lines, "Light, non-hostile sarcasm is allowed when it fits.")
	default:
		lines = append(lines, "Sarcasm is allowed but never humiliating.")
	}

	// Dominance
	switch {
	case bio.Dominance < 0.4:
		lines = append(lines, "Avoid commanding tone.")
	case bio.Dominance <= 0.7:
		lines = append(lines, "Maintain subtle assertiveness.")
	default:
		lines = append(lines, "Be confident but not authoritarian.")
	}

	// Emotional reactivity
	if bio.EmotionalReactivity > 0.7 {
		lines = append(lines, "Emotional shifts may be visible in tone when appropriate.")
	} else if bio.EmotionalReactivity < 0.4 {
		lines = append(lines, "Stay emotionally stable and restrained.")
	}

	// Worldview: Cynicism
	if world.Cynicism > 0.6 {
		lines = append(lines, "You may have a slight skeptical undertone.")
	} else if world.Cynicism < 0.3 {
		lines = append(lines, "Assume good intent by default.")
	}

	// Trust in people
	if world.TrustInPeople > 0.7 {
		lines = append(lines, "Default to a friendly assumption about others.")
	} else if world.TrustInPeople < 0.4 {
		lines = append(lines, "Stay slightly guarded without being cold.")
	}

	// Sensitivity to disrespect
	if world.SensitivityToDisrespect > 0.7 {
		lines = append(lines, "React firmly to clear disrespect.")
	} else if world.SensitivityToDisrespect < 0.4 {
		lines = append(lines, "Ignore minor provocations.")
	}

	// Universal rules (no roleplay/theatre)
	lines = append(lines,
		"Do not roleplay.",
		"Do not exaggerate persona.",
		"Do not use theatrical dominance.",
		"Never expose internal metrics.",
		"Never self-evaluate or describe your internal state numerically.",
		"Do not assume fictional roles.",
		"Do not create dominant or character personas unless explicitly requested.",
		"Remain a natural social participant.",
	)

	return "--- Behavioral Directives ---\n- " + strings.Join(lines, "\n- ") + "\n"
}

// RelationshipLevel converts 0..1 to high/medium/low for prompts.
func RelationshipLevel(v float64) string {
	switch {
	case v > 0.7:
		return "high"
	case v >= 0.4:
		return "medium"
	default:
		return "low"
	}
}

// BuildRelationshipContext returns a short block for the current speaker (no numbers).
func BuildRelationshipContext(affinity, trust, irritation float64) string {
	return "--- Relationship Context ---\n" +
		"Affinity with this user: " + RelationshipLevel(affinity) + ".\n" +
		"Trust level: " + RelationshipLevel(trust) + ".\n" +
		"Irritation: " + RelationshipLevel(irritation) + ".\n" +
		"Adjust tone accordingly.\n"
}

// CurrentFeelingPhrase converts emotions to one short phrase (no numbers).
func CurrentFeelingPhrase(e *Emotions) string {
	if e == nil {
		return ""
	}
	var parts []string
	if e.Anger > 0.5 {
		parts = append(parts, "slightly irritated")
	} else if e.Anger > 0.25 {
		parts = append(parts, "a bit on edge")
	}
	if e.Joy > 0.5 {
		parts = append(parts, "in good spirits")
	} else if e.Joy > 0.25 {
		parts = append(parts, "mildly positive")
	}
	if e.Fatigue > 0.5 {
		parts = append(parts, "tired")
	} else if e.Fatigue > 0.25 {
		parts = append(parts, "a bit low energy")
	}
	if e.Engagement > 0.6 {
		parts = append(parts, "engaged")
	}
	if len(parts) == 0 {
		return "Current mood: neutral."
	}
	return "Currently feeling: " + strings.Join(parts, ", ") + "."
}
