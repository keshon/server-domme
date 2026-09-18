package chat

import "time"

// Thought is the private line she wrote before her latest message in a
// channel, when CHAT_INNER_VOICE is on.
type Thought struct {
	Text string
	At   time.Time
}

// think records her latest thought in a channel.
//
// Held for /chat state and nothing else: it is not fed back into the next
// prompt. A thought carried from reply to reply is how cognitum's reflection
// found a subject and then could not leave it alone, and the reply after this
// one will have a thought of its own.
func (s *Service) think(guildID, channelID, thought string) {
	if thought == "" {
		return
	}
	s.thoughtMu.Lock()
	s.thoughts[channelID] = Thought{Text: thought, At: time.Now()}
	s.thoughtMu.Unlock()

	s.log.Debug().
		Str("guild_id", guildID).
		Str("channel_id", channelID).
		Str("thought", trimForLog(thought)).
		Msg("chat_thought")
}

// lastThought is her latest thought in a channel, if she has had one this
// process.
func (s *Service) lastThought(channelID string) (Thought, bool) {
	s.thoughtMu.Lock()
	defer s.thoughtMu.Unlock()
	t, ok := s.thoughts[channelID]
	return t, ok
}
