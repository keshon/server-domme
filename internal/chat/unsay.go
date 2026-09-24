package chat

import (
	"errors"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Why a message could not be taken back.
var (
	// ErrNotHers is a message somebody else wrote. Deleting those is
	// moderation, and moderation is /purge with its allowlist; this is for
	// taking back her own words.
	ErrNotHers = errors.New("chat: that message is not hers")
	// ErrNoMessage is an id Discord does not know in that channel.
	ErrNoMessage = errors.New("chat: no such message in this channel")
)

// Unsaid is what taking a message back came to.
type Unsaid struct {
	// Text is what the message said, for telling the operator what went.
	Text string
	// FromConversation is that it was still part of the conversation she is
	// in, and is not any more.
	FromConversation bool
	// Forgotten is how many moments in her memory went with it.
	Forgotten int
}

// Unsay deletes one of her own messages and takes it out of everything that
// would let it reach her next reply: the live conversation, and what she
// remembers saying. She keeps no record that it was deleted, because there
// is nothing useful she could do with one — the operator's record is the
// journal entry that says she posted it.
//
// Only her own messages: see ErrNotHers.
func (s *Service) Unsay(sess *discordgo.Session, guildID, channelID, messageID string) (Unsaid, error) {
	var out Unsaid
	if sess == nil || channelID == "" || messageID == "" {
		return out, ErrNoMessage
	}
	msg, err := sess.ChannelMessage(channelID, messageID)
	if err != nil {
		var rest *discordgo.RESTError
		if errors.As(err, &rest) && rest.Response != nil && rest.Response.StatusCode == 404 {
			return out, ErrNoMessage
		}
		return out, err
	}
	if self := selfID(sess); self == "" || msg.Author == nil || msg.Author.ID != self {
		return out, ErrNotHers
	}
	out.Text = strings.TrimSpace(msg.Content)

	if err := sess.ChannelMessageDelete(channelID, messageID); err != nil {
		return out, err
	}
	out.FromConversation = s.conv.Unsay(channelID, messageID)
	forgotten, err := s.memory.ForgetSaid(guildID, messageID)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_memory_write_failed")
	}
	out.Forgotten = forgotten

	s.log.Info().
		Str("guild_id", guildID).
		Str("channel_id", channelID).
		Str("message_id", messageID).
		Int("forgotten", forgotten).
		Msg("chat_message_unsaid")
	return out, nil
}
