package storage

import (
	"fmt"
	"time"
)

// chatBodyKey is the one row of ChatBody: she has one body across every
// server the bot is in.
const chatBodyKey = "bot"

// ChatBody is the persona's body as it stood when last saved: sleep
// pressure, energy, presence, and the approaches she missed while away.
// Operational rather than mental, which is why it lives here and not in her
// memory files. See internal/body and docs/persona-v3.md, B6.
type ChatBody struct {
	S         float64   `json:"s"`
	B         float64   `json:"b"`
	Presence  string    `json:"presence"`
	Since     time.Time `json:"since"`
	WokeAt    time.Time `json:"woke_at,omitempty"`
	AwayUntil time.Time `json:"away_until,omitempty"`
	Session   int       `json:"session"`
	Pending   bool      `json:"pending,omitempty"`
	Woken     bool      `json:"woken,omitempty"`
	HeldUntil time.Time `json:"held_until,omitempty"`
	At        time.Time `json:"at"`
	// Missed are direct approaches that came while she was not online,
	// waiting for her to come back to them.
	Missed []MissedApproach `json:"missed,omitempty"`
}

func (c *ChatBody) Key() string { return chatBodyKey }

// MissedApproach is someone speaking to her while she was away or asleep.
type MissedApproach struct {
	GuildID   string    `json:"guild_id"`
	ChannelID string    `json:"channel_id"`
	MessageID string    `json:"message_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Trigger   string    `json:"trigger"`
	At        time.Time `json:"at"`
}

// ChatBodyState returns the saved body, and false when there is none.
func (s *Storage) ChatBodyState() (ChatBody, bool) {
	got, ok := s.chatBody.Get(chatBodyKey)
	if !ok {
		return ChatBody{}, false
	}
	return *got, true
}

// SetChatBodyState saves the body.
func (s *Storage) SetChatBodyState(c ChatBody) error {
	if err := s.chatBody.Put(&c); err != nil {
		return fmt.Errorf("storage: set chat body: %w", err)
	}
	return nil
}
