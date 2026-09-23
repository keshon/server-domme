package chat

import "github.com/bwmarrin/discordgo"

// AgeRestricted reports whether Discord marks a channel age-restricted, or it
// is a thread in one. Whether that closes it to her is the service's to say:
// see Service.closed and CHAT_AGE_RESTRICTED.
//
// The server's own marking, rather than a judgement of what is said: a
// server keeps its explicit content in age-restricted channels, because
// Discord requires it to, and that line is one the code can follow without
// reading anything. What she keeps from everywhere else is her own SFW
// gist; see mind.idleRules and the card's Avoid.
//
// A channel the session does not know is not restricted as far as this can
// tell: the state holds every channel of every guild the bot is in, and
// refusing on a cache miss would silence her whenever the cache is cold.
func AgeRestricted(sess *discordgo.Session, channelID string) bool {
	if sess == nil || sess.State == nil || channelID == "" {
		return false
	}
	ch, err := sess.State.Channel(channelID)
	if err != nil || ch == nil {
		return false
	}
	if ch.NSFW {
		return true
	}
	if ch.IsThread() && ch.ParentID != "" {
		parent, err := sess.State.Channel(ch.ParentID)
		return err == nil && parent != nil && parent.NSFW
	}
	return false
}

// closed reports whether a channel is closed to her whatever mode it was
// given: age-restricted, while she is not allowed in those. Nothing is taken
// from a closed channel and nothing is said in it — not read, not walked
// through, not answered, not started in.
//
// With CHAT_AGE_RESTRICTED on, an age-restricted channel is like any other:
// it still has to be opted in one channel at a time, and what is said there
// still goes to the relays, which may refuse it.
func (s *Service) closed(sess *discordgo.Session, channelID string) bool {
	return !s.ageRestricted && AgeRestricted(sess, channelID)
}

// MayReadRestricted reports whether she may be let into age-restricted
// channels at all: CHAT_AGE_RESTRICTED. For the command that opts one in.
func (s *Service) MayReadRestricted() bool { return s != nil && s.ageRestricted }
