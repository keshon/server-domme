package chat

import "github.com/keshon/server-domme/internal/mind"

// perceptionUnreadable is what the journal records when the model was asked
// for a label and gave nothing usable. Kept distinct from "not asked", since
// how often the relays fail to label at all is half of what shadow mode is
// for.
const perceptionUnreadable = "unreadable"

// perceived takes the model's label for their message out of a reply,
// records it, and returns what is left to post, reporting false when nothing
// is.
//
// Shadow only: the label goes in the journal and the log and moves nothing.
// See mind.Perception for why it waits there.
func (s *Service) perceived(t task, sp *spoken, reply string) (string, bool) {
	label, message, ok := mind.SplitPerception(reply)
	sp.perceived = string(label)
	if label == "" {
		sp.perceived = perceptionUnreadable
	}
	s.log.Info().
		Str("guild_id", t.item.GuildID).
		Str("channel_id", t.item.ChannelID).
		Str("user_id", t.item.UserID).
		Str("perceived", sp.perceived).
		Msg("chat_perceived")
	return message, ok
}
