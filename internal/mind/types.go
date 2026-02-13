package mind

import "time"

// Biology — immutable core parameters. LLM must never change this.
// JSON keys kept readable for future LLM summaries (e.g. trust_in_people).
type Biology struct {
	Temperament string  `json:"temperament"` // e.g. "dominant_reserved"
	Age         int     `json:"age"`
	SpeechStyle string  `json:"speech_style"` // e.g. "sharp_aristocratic"
	Dominance   float64 `json:"dominance"`   // 0..1
	EmoReact    float64 `json:"emotional_reactivity"` // 0..1
}

// Worldview — evolvable beliefs. Small deltas only, validated by code.
type Worldview struct {
	TrustInPeople float64 `json:"trust_in_people"` // 0..1
	Cynicism      float64 `json:"cynicism"`        // 0..1
	Openness      float64 `json:"openness"`        // 0..1
	LoyaltyBias   float64 `json:"loyalty_bias"`    // 0..1
	UpdatedAt     string  `json:"updated_at,omitempty"`
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
	Score         float64   `json:"score"`           // ActivityScore, decay over time
	LastMsgAt     time.Time `json:"last_msg_at"`
	LastTickAt    time.Time `json:"last_tick_at,omitempty"`
	LastSpokeAt   time.Time `json:"last_spoke_at,omitempty"`
	LastChannelID string    `json:"last_channel_id,omitempty"` // where to send reply
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
