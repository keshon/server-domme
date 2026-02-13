// Package mind implements the cognitive layer for a proactive Discord agent.
//
// Logical structure (by file): core (biology, identity, worldview), guild (state, short buffer, activity),
// scheduler (one goroutine, priority by guild), decision (DesireToSpeak), limiter (LLM rate limits),
// memory (summarization), emotions (decay, bump), people (PersonUpdateEngine), evolution (worldview),
// context (TokenBudget, BuildMessagesForLLM), runner (wire to Discord + AI).
//
// LLM is used only for: speech generation, rare memory summarization, rare worldview evolution. Rest is deterministic Go.
package mind

import "time"

// Temperament — Big Five (0..1). Part of Biology.
type Temperament struct {
	Openness         float64 `json:"openness"`
	Conscientiousness float64 `json:"conscientiousness"`
	Extraversion    float64 `json:"extraversion"`
	Agreeableness   float64 `json:"agreeableness"`
	Neuroticism     float64 `json:"neuroticism"`
}

// SpeechStyle — 0..1. Part of Biology.
type SpeechStyle struct {
	Verbosity float64 `json:"verbosity"`
	Sarcasm   float64 `json:"sarcasm"`
	Formality float64 `json:"formality"`
	Warmth    float64 `json:"warmth"`
}

// Biology — immutable core parameters. LLM must never change this.
type Biology struct {
	Temperament        Temperament `json:"temperament"`
	Dominance          float64     `json:"dominance"`
	EmotionalReactivity float64     `json:"emotional_reactivity"`
	BaselineEnergy     float64     `json:"baseline_energy"`
	BaselineEngagement float64     `json:"baseline_engagement"`
	SpeechStyle        SpeechStyle `json:"speech_style"`
	ConflictTendency   float64     `json:"conflict_tendency"`
	LoyaltyBias        float64     `json:"loyalty_bias"`
	CuriosityDrive     float64     `json:"curiosity_drive"`
	Adaptability       float64     `json:"adaptability"`
	Impulsivity        float64     `json:"impulsivity"`
}

// Worldview — evolvable beliefs. Small deltas only, validated by code.
type Worldview struct {
	TrustInPeople              float64 `json:"trust_in_people"`
	Cynicism                   float64 `json:"cynicism"`
	Optimism                   float64 `json:"optimism"`
	Patience                   float64 `json:"patience"`
	Skepticism                 float64 `json:"skepticism"`
	AttachmentToRegulars       float64 `json:"attachment_to_regulars"`
	SensitivityToDisrespect    float64 `json:"sensitivity_to_disrespect"`
	NeedForRecognition         float64 `json:"need_for_recognition"`
	ToleranceForChaos          float64 `json:"tolerance_for_chaos"`
	RiskTaking                 float64 `json:"risk_taking"`
	ValueOfLoyalty             float64 `json:"value_of_loyalty"`
	ImportanceOfIntellectualDepth float64 `json:"importance_of_intellectual_depth"`
	UpdatedAt                  string  `json:"updated_at,omitempty"`
}

// Emotions — current state per guild. Decay over time, boosted by events.
type Emotions struct {
	Anger     float64 `json:"anger"`     // 0..1
	Joy       float64 `json:"joy"`       // 0..1
	Fatigue   float64 `json:"fatigue"`   // 0..1
	Engagement float64 `json:"engagement"` // 0..1
	UpdatedAt string  `json:"updated_at,omitempty"`
}

// Person — per-user model within a guild.
type Person struct {
	UserID   string  `json:"user_id"`
	Respect  float64 `json:"respect"`   // 0..1
	Trust    float64 `json:"trust"`     // 0..1
	Irritation float64 `json:"irritation"` // 0..1
	Affinity float64 `json:"affinity"`  // 0..1
	Summary  string  `json:"summary"`   // short text summary
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Activity — guild activity state for scheduler.
type Activity struct {
	Score                float64   `json:"score"`
	LastActivityUpdate   time.Time `json:"last_activity_update"`
	LastMsgAt             time.Time `json:"last_msg_at"`
	LastTickAt            time.Time `json:"last_tick_at,omitempty"`
	LastSpokeAt          time.Time `json:"last_spoke_at,omitempty"`
	LastLLMCallAt         time.Time `json:"last_llm_call_at,omitempty"`
	LastChannelID        string    `json:"last_channel_id,omitempty"`
	ConsecutiveBotReplies int      `json:"consecutive_bot_replies,omitempty"`
	// PendingResponse: bot asked a question and is waiting for user reply; block proactive until reply or timeout
	AwaitingReply     bool      `json:"awaiting_reply,omitempty"`
	AwaitingReplySince time.Time `json:"awaiting_reply_since,omitempty"`
	AwaitingTopic     string    `json:"awaiting_topic,omitempty"`
	// LastAIIntent: avoid proactive repeating the same question/action
	LastAIAction    string    `json:"last_ai_action,omitempty"`
	LastAITopic     string    `json:"last_ai_topic,omitempty"`
	LastAITimestamp time.Time `json:"last_ai_timestamp,omitempty"`
}

// ShortMessage — one message in short-term buffer (for context / decision).
type ShortMessage struct {
	Role      string    `json:"role"`      // "user" | "assistant"
	UserID    string    `json:"user_id,omitempty"`
	Username  string    `json:"username,omitempty"`
	Content   string    `json:"content"`
	ChannelID string    `json:"channel_id"`
	At        time.Time `json:"at"`
	Mentioned bool      `json:"mentioned"` // bot was mentioned
}

// CorePaths — paths under data/mind/core/.
const (
	CoreBiology   = "biology.json"
	CoreIdentity  = "identity.md"
	CoreWorldview = "worldview.json"
)

// GuildFilenames — under data/mind/guilds/{guildID}/.
const (
	GuildMediumMemory = "medium_memory.md"
	GuildEmotions     = "emotions.json"
	GuildActivity     = "activity.json"
)

// ActivityDecayK — коэффициент экспоненциального затухания: score *= exp(-k * elapsedSeconds).
const ActivityDecayK = 0.015

// ShortMemorySummarizeThreshold — при shortCharCount > этого значения вызывается суммаризация.
const ShortMemorySummarizeThreshold = 12000

// AwaitingReplyTimeout — после вопроса не проактивно говорить, пока пользователь не ответит или не истечёт таймаут.
const AwaitingReplyTimeout = 2 * time.Minute

// MemoryEntry — эпизодическая память: конкретное событие/разговор для глубины развития.
type MemoryEntry struct {
	Timestamp       string   `json:"timestamp"`
	Type            string   `json:"type"` // e.g. "conversation_event"
	Summary         string   `json:"summary"`
	Participants    []string `json:"participants,omitempty"`
	Topics          []string `json:"topics,omitempty"`
	EmotionalWeight float64  `json:"emotional_weight"`
	Importance      float64  `json:"importance"`
}
