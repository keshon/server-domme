package mind

import (
	"math/rand"
	"time"
)

// DecisionConfig holds thresholds and weights for DesireToSpeak.
type DecisionConfig struct {
	SpeakThreshold   float64       // min DesireToSpeak to trigger LLM (e.g. 0.4)
	ActivityWeight  float64       // weight for ActivityScore (0..1 scale)
	EmotionWeight   float64
	TopicWeight     float64
	RecentSpokePenalty float64    // subtract if spoke recently
	RecentSpokeWindow  time.Duration
	RandomFactor    float64       // [0, RandomFactor] added
}

// DefaultDecisionConfig returns a sane default.
func DefaultDecisionConfig() DecisionConfig {
	return DecisionConfig{
		SpeakThreshold:     0.35,
		ActivityWeight:     0.3,
		EmotionWeight:      0.25,
		TopicWeight:        0.25,
		RecentSpokePenalty: 0.3,
		RecentSpokeWindow:  2 * time.Minute,
		RandomFactor:       0.15,
	}
}

// DesireToSpeak computes 0..1 score. No LLM — pure Go.
func DesireToSpeak(cfg DecisionConfig, activityScore float64, emotionalActivation float64, topicRelevance float64, lastSpokeAt time.Time, now time.Time) float64 {
	// Normalize activity to ~0..1 (ActivityScore is 0..100)
	actNorm := activityScore / 100.0
	if actNorm > 1 {
		actNorm = 1
	}

	score := cfg.ActivityWeight*actNorm +
		cfg.EmotionWeight*emotionalActivation +
		cfg.TopicWeight*topicRelevance

	// Recently spoke -> penalty
	if !lastSpokeAt.IsZero() && now.Sub(lastSpokeAt) < cfg.RecentSpokeWindow {
		score -= cfg.RecentSpokePenalty
	}

	// Random factor (so bot doesn't always speak at same threshold)
	score += cfg.RandomFactor * rand.Float64()

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
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
