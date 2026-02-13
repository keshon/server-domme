package mind

import (
	"math/rand"
	"time"
)

// DecisionConfig holds thresholds and weights for DesireToSpeak.
type DecisionConfig struct {
	SpeakThreshold      float64
	ActivityWeight      float64 // w1
	EmotionWeight       float64 // w2
	TopicWeight         float64 // w3
	FatigueWeight       float64 // w4, subtract
	RecentSpokePenalty  float64 // w5
	RecentSpokeWindow   time.Duration
	RandomFactor        float64
	RunawayMaxConsecutive int   // max bot messages in a row without user reply (e.g. 3)
}

// MentionBoost is added to desire when the bot was recently mentioned (direct address).
const MentionBoost = 0.15

// DefaultDecisionConfig returns a sane default. Threshold 0.28 so bot reacts to direct address.
func DefaultDecisionConfig() DecisionConfig {
	return DecisionConfig{
		SpeakThreshold:       0.28,
		ActivityWeight:        0.25,
		EmotionWeight:        0.2,
		TopicWeight:          0.2,
		FatigueWeight:        0.1,
		RecentSpokePenalty:   0.25,
		RecentSpokeWindow:    2 * time.Minute,
		RandomFactor:         0.12,
		RunawayMaxConsecutive: 3,
	}
}

// DesireToSpeakInput bundles all inputs for the decision (no LLM — pure Go).
type DesireToSpeakInput struct {
	ActivityScoreNorm     float64
	EmotionalActivation   float64
	TopicRelevance        float64
	Fatigue               float64
	LastSpokeAt           time.Time
	ConsecutiveBotReplies int
	HasRecentMention      bool
	Now                   time.Time
}

// DesireToSpeak computes 0..1 score. Mention boost applied when HasRecentMention.
func DesireToSpeak(cfg DecisionConfig, in DesireToSpeakInput) float64 {
	if in.ConsecutiveBotReplies >= cfg.RunawayMaxConsecutive {
		return 0
	}

	score := cfg.ActivityWeight*in.ActivityScoreNorm +
		cfg.EmotionWeight*in.EmotionalActivation +
		cfg.TopicWeight*in.TopicRelevance -
		cfg.FatigueWeight*in.Fatigue

	if in.HasRecentMention {
		score += MentionBoost
	}

	if !in.LastSpokeAt.IsZero() && in.Now.Sub(in.LastSpokeAt) < cfg.RecentSpokeWindow {
		score -= cfg.RecentSpokePenalty
	}

	score += cfg.RandomFactor * rand.Float64()

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

// HasRecentMention returns true if any message in the buffer within since has Mentioned set.
func HasRecentMention(msgs []ShortMessage, since time.Duration, now time.Time) bool {
	cutoff := now.Add(-since)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].At.Before(cutoff) {
			break
		}
		if msgs[i].Mentioned {
			return true
		}
	}
	return false
}

// TopicRelevanceFromBuffer returns 0..1 from recent messages: mention = high, else lower.
func TopicRelevanceFromBuffer(msgs []ShortMessage, since time.Duration) float64 {
	if len(msgs) == 0 {
		return 0
	}
	cutoff := time.Now().Add(-since)
	var mentioned, total int
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].At.Before(cutoff) {
			break
		}
		total++
		if msgs[i].Mentioned {
			mentioned++
		}
	}
	if total == 0 {
		return 0
	}
	// If any recent message mentioned us, high relevance
	if mentioned > 0 {
		return 0.7 + 0.3*float64(mentioned)/float64(total)
	}
	// Else low baseline (conversation in channel)
	return 0.2
}
