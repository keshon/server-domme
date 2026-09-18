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

	if t.item.Trigger == mind.TriggerAfterthought && !s.afterthoughtStands(t, reply) {
		return
	}

	// A copy of someone's line is a failed generation that happened to parse,
	// so it takes the same path: an answer is tried again later, something
	// volunteered is dropped.
	if mind.Echoes(reply, s.conv.Recent(t.item.ChannelID)) {
		s.log.Warn().
			Str("guild_id", t.item.GuildID).
			Str("channel_id", t.item.ChannelID).
			Msg("chat_reply_echoed")
		s.hold(t, "echoed")
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
	s.considerAfterthought(ctx, t, grounding, reply, sentID, spokeAt)

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

// maxRecalled is how many memories go in front of her at once.
//
// Small on purpose. The point of recall is that something relevant surfaces,
// not that she arrives holding a dossier; a character who lists everything she
// remembers about a room is doing something no person does.
const maxRecalled = 3

// remember returns what the room is currently talking about and what she still
// recalls of it.
//
// The topic is the live conversation's own words, which is what decides whether
// an old memory is close enough to the subject to come back — cognitum's P6,
// done with set overlap rather than embeddings because embeddings would need a
// model call per memory per message.
func (s *Service) remember(guildID, channelID string, present []mind.Acquaintance, now time.Time) (string, []mind.Memory) {
	var topic strings.Builder
	for _, turn := range s.conv.Recent(channelID) {
		topic.WriteString(turn.Content)
		topic.WriteString(" ")
	}

	memories := s.memoriesOf(guildID, channelID)
	if len(memories) == 0 {
		return topic.String(), nil
	}

	here := make([]string, 0, len(present))
	for _, p := range present {
		here = append(here, p.UserID)
	}

	return topic.String(), mind.Recall(memories, now, mind.Keywords(topic.String()), here, maxRecalled)
}

// standing is what this person's roles mean to her: one directive and the
// combined regard.
//
// Roles come from the gateway's own cached member, which is populated for
// anyone who has spoken. A member it cannot resolve has no roles as far as
// this is concerned, and no standing either way — the safe reading, since the
// alternative is treating someone as a stranger because a cache missed.
func (s *Service) standing(sess *discordgo.Session, guildID, userID, username string) (string, float64) {
	if sess == nil || guildID == "" || userID == "" {
		return "", 0
	}

	biases := s.store.ChatRoleBiases(guildID)
	if len(biases) == 0 {
		return "", 0
	}

	member, err := sess.State.Member(guildID, userID)
	if err != nil || member == nil {
		return "", 0
	}

	var values []float64
	var note string
	var strongest float64

	for _, roleID := range member.Roles {
		bias, ok := biases[roleID]
		if !ok {
			continue
		}
		values = append(values, bias.Regard)

		// One note, from whichever role counts for most. Several would be a
		// list of opinions about one person in a prompt that has to fit.
		if abs(bias.Regard) >= abs(strongest) && strings.TrimSpace(bias.Note) != "" {
			note, strongest = bias.Note, bias.Regard
		}
	}
	if len(values) == 0 {
		return "", 0
	}

	regard := mind.Combine(values)
	return mind.RegardDirective(username, note, regard), regard
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// hold puts an approach back for a later attempt, or gives up on it.
//
// Giving up quietly is the point: she simply never answers, which is a thing
// people do. The alternative — telling the channel that a backend is
// unavailable — breaks character to report plumbing nobody there can fix.
func (s *Service) hold(t task, reason string) {
	// Only an answer is held. Something she volunteered, or a second thought
	// about her own reply, belongs to its moment; see mind.Owed.
	if !mind.Owed(t.item.Trigger) {
		s.log.Info().
			Str("guild_id", t.item.GuildID).
			Str("channel_id", t.item.ChannelID).
			Str("reason", reason).
			Str("trigger", string(t.item.Trigger)).
			Msg("chat_unowed_dropped")
		return
	}
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

	// Anchored when a bare message would leave people guessing: a held reply,
	// an answer to someone who replied to her, or a channel where somebody
	// else has spoken since the line she is answering. Not otherwise — see
	// mind.NeedsAnchor.
	//
	// An afterthought never is. It follows her own message, and quoting the
	// original line again would split one thought across two anchors.
	anchor := t.late || t.item.Trigger == mind.TriggerReply ||
		mind.NeedsAnchor(s.conv.Recent(t.item.ChannelID), t.item.MessageID, t.item.UserID)
	if anchor && t.item.Trigger != mind.TriggerAfterthought {
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
	g.Topic, g.Remembers = s.remember(t.item.GuildID, t.item.ChannelID, g.Present, now)

	// About the person being answered, not about everyone present: standing is
	// a fact about one member, and a paragraph covering the room would be more
	// prompt than it is worth and harder to act on.
	g.AboutThem, g.Regard = s.standing(sess, t.item.GuildID, t.item.UserID, t.item.Username)
	g.Volunteering = t.item.Volunteering
	if t.item.Trigger == mind.TriggerAfterthought {
		g.Afterthought = mind.AfterthoughtDirective(t.item.FirstLine)
	}
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
			who.Irritation = mind.IrritationNow(known.Irritation, known.IrritatedAt, time.Now())
			if known.Username != "" {
				who.Username = known.Username
			}
		}
		out = append(out, who)
	}
	return out
}

// regardFor is the combined standing of someone's roles, for the decision to
// answer at all. The directive that goes with it is built later, at reply
// time; see Service.standing.
func (s *Service) regardFor(sess *discordgo.Session, guildID, userID string) float64 {
	_, regard := s.standing(sess, guildID, userID, "")
	return regard
}

// memoriesOf loads what she remembers about a channel, as the mind package
// reads it. Shared by reply-time recall and by volunteering, so the two cannot
// drift into disagreeing about what she knows.
func (s *Service) memoriesOf(guildID, channelID string) []mind.Memory {
	stored := s.store.MindMemories(guildID, channelID)
	out := make([]mind.Memory, 0, len(stored))
	for _, m := range stored {
		out = append(out, mind.Memory{
			At:     m.At,
			Gist:   m.Gist,
			Detail: m.Detail,
			Weight: m.Weight,
			People: m.People,
		})
	}
	return out
}

// liveTopic is the words of the conversation in progress, run together.
func (s *Service) liveTopic(channelID string) string {
	var topic strings.Builder
	for _, turn := range s.conv.Recent(channelID) {
		topic.WriteString(turn.Content)
		topic.WriteString(" ")
	}
	return topic.String()
}
