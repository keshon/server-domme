package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/mind"
)

// speak builds and sends one reply, holding the approach for a later attempt
// if no backend will answer.
//
// A failure here is deliberately silent in the channel. The backends are
// donated public relays that fail routinely, and announcing that in a
// conversation ("no backend available") is the most machine-like thing the bot
// could do — it breaks the character to report its own plumbing. Someone who
// was busy answers late instead; see mind.Deferred.
func (s *Service) speak(ctx context.Context, t task) {
	sess := s.session()
	if sess == nil {
		s.hold(t, "no session")
		return
	}

	// Typing shows on the first attempt only.
	//
	// It is an honest signal that something is coming, and costs nothing when
	// the reply then takes twenty seconds. On a retry it is a lie: the last
	// attempt failed and this one may too. Shown every time, a single
	// unanswerable message had the bot appearing to type on and off for the
	// whole deferral window, which is how an outage reads as a haunting.
	if !t.late {
		if err := sess.ChannelTyping(t.item.ChannelID); err != nil {
			s.log.Debug().Err(err).Str("channel_id", t.item.ChannelID).Msg("chat_typing_failed")
		}
	}

	// Read the channel's own history the first time she speaks here, so the
	// reply answers the conversation rather than the one line that triggered
	// it. Costs one REST call per channel per process; see backfill.
	s.backfill(sess, t.item.ChannelID)

	grounding := s.ground(sess, t)
	messages := mind.Build(s.character, grounding, s.conv.Recent(t.item.ChannelID), s.budget)

	genCtx, cancel := context.WithTimeout(ctx, s.generateTimeout)
	defer cancel()

	reply, err := s.provider.Generate(genCtx, messages)
	if err != nil {
		// A cancelled root context is a shutdown, not a backend problem.
		// Holding the approach then would be recording work for a process
		// that is going away.
		if ctx.Err() != nil {
			return
		}
		s.log.Warn().
			Err(err).
			Str("guild_id", t.item.GuildID).
			Int("attempts", t.item.Attempts).
			Bool("no_backend", errors.Is(err, ai.ErrNoBackend)).
			Msg("chat_generate_failed")
		s.hold(t, "generate failed")
		return
	}

	sent, err := s.send(sess, t, reply)
	if err != nil {
		// The reply exists but could not be delivered — a missing permission,
		// a deleted channel. Retrying would most likely fail the same way, so
		// this is logged and dropped rather than held.
		s.log.Warn().
			Err(err).
			Str("guild_id", t.item.GuildID).
			Str("channel_id", t.item.ChannelID).
			Msg("chat_send_failed")
		return
	}

	// The id is what lets a later reply pointing at this message be recognised
	// as a reply to her; see Service.repliesToHer.
	var sentID string
	if sent != nil {
		sentID = sent.ID
	}
	spokeAt := time.Now()
	s.conv.Record(t.item.ChannelID, mind.Turn{
		Content:   reply,
		At:        spokeAt,
		FromBot:   true,
		MessageID: sentID,
	})
	// Persisted because the conversation buffer keeps half an hour and the
	// social drive is measured in hours; see Service.drives.
	if err := s.store.MarkMindSpoke(t.item.GuildID, spokeAt); err != nil {
		s.log.Warn().Err(err).Str("guild_id", t.item.GuildID).Msg("chat_spoke_record_failed")
	}
	s.deferrals.Drop(t.item.ChannelID)

	s.log.Info().
		Str("guild_id", t.item.GuildID).
		Str("channel_id", t.item.ChannelID).
		Str("trigger", string(t.item.Trigger)).
		Bool("late", t.late).
		Int("chars", len(reply)).
		Msg("chat_replied")
}

// drives computes how she is doing, from the clock and from state already to
// hand.
//
// No backend call and no stored mood: the drives are derived on read from a
// timestamp and two counts, so they cost nothing, cannot drift out of step with
// what actually happened, and survive a restart because the timestamp does.
func (s *Service) drives(guildID, channelID string, now time.Time) mind.Drives {
	in := mind.MoodInput{Now: now, Location: s.location}

	if guild := s.store.GetMindGuild(guildID); guild != nil {
		in.LastSpokeAt = guild.LastSpokeAt
	}

	for _, turn := range s.conv.Recent(channelID) {
		in.RecentTurns++
		if turn.Mentioned {
			in.AddressedTurns++
		}
	}
	return mind.DeriveDrives(in)
}

// hold puts an approach back for a later attempt, or gives up on it.
//
// Giving up quietly is the point: she simply never answers, which is a thing
// people do. The alternative — telling the channel that a backend is
// unavailable — breaks character to report plumbing nobody there can fix.
func (s *Service) hold(t task, reason string) {
	if s.deferrals.Hold(t.item, time.Now()) {
		return
	}
	s.log.Info().
		Str("guild_id", t.item.GuildID).
		Str("channel_id", t.item.ChannelID).
		Int("attempts", t.item.Attempts).
		Str("reason", reason).
		Msg("chat_approach_abandoned")
}

// send posts the reply, anchored to the message it answers.
//
// It goes out as a Discord reply for two of the three triggers: answering a
// reply, and any late answer. A late answer that is not anchored reads as an
// interruption about nothing, because by then the thing it answers has
// scrolled away — the anchor is what makes the delay work rather than just
// being a delay.
func (s *Service) send(sess *discordgo.Session, t task, content string) (*discordgo.Message, error) {
	msg := &discordgo.MessageSend{
		Content: content,
		// She may address people by name, but nothing she says should ping a
		// role or the whole server. Replies do not ping the author either: a
		// notification for every line of a conversation someone is already
		// reading is noise.
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
		},
	}

	if t.late || t.item.Trigger == mind.TriggerReply {
		msg.Reference = &discordgo.MessageReference{
			MessageID: t.item.MessageID,
			ChannelID: t.item.ChannelID,
			GuildID:   t.item.GuildID,
		}
	}

	return sess.ChannelMessageSendComplex(t.item.ChannelID, msg)
}

// ground assembles what she knows about where she is.
//
// Everything comes from the session's own cache rather than from the API: this
// runs per reply, and a guild plus channel fetch each time would spend rate
// limit on facts the gateway already sent. A cache miss renders as a missing
// line, which Grounding.Render is built to tolerate.
func (s *Service) ground(sess *discordgo.Session, t task) mind.Grounding {
	now := time.Now()

	names := s.namesFor(sess, t.item.GuildID)
	g := mind.Grounding{
		SelfName: s.DisplayName(sess, t.item.GuildID),
		Brief:    s.store.GetChatBrief(t.item.GuildID),
		Now:      now,
	}
	// The display name is already the identity line; listing it again as an
	// alias of itself reads as confusion rather than as thoroughness.
	for _, name := range names {
		if !strings.EqualFold(name, g.SelfName) {
			g.SelfAliases = append(g.SelfAliases, name)
		}
	}

	if guild, err := sess.State.Guild(t.item.GuildID); err == nil && guild != nil {
		g.GuildName = guild.Name
	}
	if channel, err := sess.State.Channel(t.item.ChannelID); err == nil && channel != nil {
		g.ChannelName = channel.Name
		g.ChannelTopic = channel.Topic
	}

	if t.late {
		g.AnsweringAfter = t.item.Age(now)
	}

	g.Present = s.present(t.item.GuildID, t.item.ChannelID)
	g.Drives = s.drives(t.item.GuildID, t.item.ChannelID, now)
	return g
}

// present lists the people in the live conversation, most recent speaker last.
func (s *Service) present(guildID, channelID string) []mind.Acquaintance {
	seen := make(map[string]bool)
	var out []mind.Acquaintance

	for _, turn := range s.conv.Recent(channelID) {
		if turn.FromBot || turn.UserID == "" || seen[turn.UserID] {
			continue
		}
		seen[turn.UserID] = true

		who := mind.Acquaintance{UserID: turn.UserID, Username: turn.Username}
		if known := s.store.GetMindPerson(guildID, turn.UserID); known != nil {
			who.Messages = known.Messages
			who.FirstSeen = known.FirstSeen
			who.LastSeen = known.LastSeen
			who.PrevSeen = known.PrevSeen
			if known.Username != "" {
				who.Username = known.Username
			}
		}
		out = append(out, who)
	}
	return out
}
