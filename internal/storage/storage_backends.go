package storage

import (
	"fmt"
	"time"
)

// chatBackendsKey is the one row of ChatBackends. The arrangement is
// bot-wide: the backends are shared by every server the bot is in.
const chatBackendsKey = "bot"

// ChatBackends is how the bot's owner has arranged the persona's backends
// with /chat backends: the order they are tried in, the ones her voice
// prefers, and the ones switched off. Names, not specs — the specs and their
// keys stay in the environment, and this only arranges what the environment
// provides.
type ChatBackends struct {
	Mode      string    `json:"mode,omitempty"`
	Order     []string  `json:"order,omitempty"`
	Voice     []string  `json:"voice,omitempty"`
	Off       []string  `json:"off,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *ChatBackends) Key() string { return chatBackendsKey }

// ChatBackendArrangement returns the stored arrangement, and false when the
// owner has never changed it.
func (s *Storage) ChatBackendArrangement() (ChatBackends, bool) {
	got, ok := s.chatBackends.Get(chatBackendsKey)
	if !ok {
		return ChatBackends{}, false
	}
	return *got, true
}

// SetChatBackendArrangement stores the arrangement.
func (s *Storage) SetChatBackendArrangement(c ChatBackends) error {
	if err := s.chatBackends.Put(&c); err != nil {
		return fmt.Errorf("storage: set chat backends: %w", err)
	}
	return nil
}

// ClearChatBackendArrangement forgets the arrangement, so the environment's
// order applies again from the next start.
func (s *Storage) ClearChatBackendArrangement() error {
	if err := s.chatBackends.Delete(chatBackendsKey); err != nil {
		return fmt.Errorf("storage: clear chat backends: %w", err)
	}
	return nil
}
