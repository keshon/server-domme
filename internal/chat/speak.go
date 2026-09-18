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
	// Whatever happens below ends up in the journal entry for this approach;
	// see closeSpoken.
	sp := &spoken{}
	defer s.closeSpoken(t, sp)
	started := time.Now()

	sess := s.session()
	if sess == nil {
		s.holdSpoken(t, sp, "no gateway session")
		return
	}

	// Typing shows on the first attempt only.
	//
	// It is an honest signal that something is coming, and costs nothing when
	// the reply then takes twenty seconds. On a retry it is a lie: the last
	// attempt failed and this one may too. Shown every time, a single
	// unanswerable message had the bot appearing to type on and off for the
	// whole deferral window, which is how an outage reads as a haunting.
	//
	// Not for an afterthought either, whose typing shows only once it is
	// certain to be sent; see typeBriefly.
	//
	// Nor for an approach she may still decline: typing that ends in nothing
	// reads as her writing something and deleting it. Those show typing once
	// the reply is certain; see typeBriefly.
	if !t.late && t.item.Trigger != mind.TriggerAfterthought && !declinable(t) {
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

	sp.told = grounding.Told()
	reply, backend, err := s.generate(genCtx, messages)
	sp.raw, sp.backend = reply, backend
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
		s.count(t.item.GuildID, countRelayFailed)
		s.holdSpoken(t, sp, "no relay answered")
		return
	}

	if grounding.InnerVoice && t.item.Trigger != mind.TriggerAfterthought && !mind.Volunteered(t.item.Trigger) {
		thought, message, ok := mind.SplitThought(reply)
		if !ok {
			// Never posted: a private thought reaching the channel cannot be
			// taken back. Asked again at once without the thought instead —
			// the usual failure is a model that wrote the thought and stopped,
			// and waiting for the deferral to retry would make a formatting
			// slip look like an outage.
			s.log.Warn().
				Str("guild_id", t.item.GuildID).
				Str("channel_id", t.item.ChannelID).
				Msg("chat_thought_unsplittable")
			grounding.InnerVoice = false
			plain := mind.Build(s.character, grounding, s.conv.Recent(t.item.ChannelID), s.budget)
			if reply, sp.backend, err = s.generate(genCtx, plain); err != nil {
				if ctx.Err() == nil {
					s.holdSpoken(t, sp, "no relay answered")
				}
				return
			}
		} else {
			reply = message
			s.think(t.item.GuildID, t.item.ChannelID, thought)
			sp.thought = thought
		}
	}

	if grounding.MayDecline && mind.IsSkip(reply) {
		// Her choice, made with the whole conversation in front of her, and
		// final like any other decision not to answer: nothing is held.
		s.log.Info().
			Str("guild_id", t.item.GuildID).
			Str("channel_id", t.item.ChannelID).
			Str("trigger", string(t.item.Trigger)).
			Bool("closer", t.item.Closer).
			Msg("chat_declined")
		s.count(t.item.GuildID, countDeclined)
		sp.outcome, sp.reason = outcomeDeclined, "decided it did not need an answer (SKIP)"
		return
	}

	if t.item.Trigger == mind.TriggerAfterthought {
		if !s.afterthoughtStands(t, reply) {
			sp.reason = "second thought declined, repeated her or was overtaken"
			return
		}
		// Every check that could still drop it runs before typing shows, so
		// typing always ends in a message.
		recent := s.conv.Recent(t.item.ChannelID)
		if _, repeats := mind.RepeatsHerself(reply, recent); repeats || mind.Echoes(reply, recent) {
			s.log.Info().Str("channel_id", t.item.ChannelID).Msg("chat_afterthought_withheld")
			sp.reason = "second thought repeated something already said"
			return
		}
		if !s.typeBriefly(ctx, sess, t) {
			sp.reason = "the conversation moved on while she typed"
			return
		}
	}

	// A copy of someone's line is a failed generation that happened to parse,
	// so it takes the same path: an answer is tried again later, something
	// volunteered is dropped.
	if mind.Echoes(reply, s.conv.Recent(t.item.ChannelID)) {
		s.log.Warn().
			Str("guild_id", t.item.GuildID).
			Str("channel_id", t.item.ChannelID).
			Msg("chat_reply_echoed")
		// Something she was free to decline is not retried later either: a
		// late answer to "same" is stranger than none.
		s.count(t.item.GuildID, countEcho)
		if grounding.MayDecline {
			sp.reason = "the model echoed their line"
		} else {
			s.holdSpoken(t, sp, "the model echoed their line")
		}
		return
	}

	if earlier, repeats := mind.RepeatsHerself(reply, s.conv.Recent(t.item.ChannelID)); repeats {
		// Asked again once, told what she already said. Not held if it
		// repeats a second time: a retry later would build the same prompt
		// and get the same line, and silence is better than a loop.
		s.log.Warn().
			Str("guild_id", t.item.GuildID).
			Str("channel_id", t.item.ChannelID).
			Msg("chat_reply_repeated")
		s.count(t.item.GuildID, countRepeat)
		if t.item.Trigger == mind.TriggerAfterthought {
			sp.reason = "second thought repeated something already said"
			return
		}
		grounding.InnerVoice = false
		again := append(mind.Build(s.character, grounding, s.conv.Recent(t.item.ChannelID), s.budget),
			ai.Message{Role: ai.RoleSystem, Content: mind.RepeatNote(earlier)})
		if reply, sp.backend, err = s.generate(genCtx, again); err != nil {
			if ctx.Err() == nil {
				s.holdSpoken(t, sp, "no relay answered")
			}
			return
		}
		if _, still := mind.RepeatsHerself(reply, s.conv.Recent(t.item.ChannelID)); still {
			s.log.Warn().
				Str("guild_id", t.item.GuildID).
				Str("channel_id", t.item.ChannelID).
				Msg("chat_repeat_dropped")
			sp.reason = "repeated herself twice; silence rather than a loop"
			return
		}
	}

	if grounding.MayDecline && !s.typeBriefly(ctx, sess, t) {
		sp.reason = "shutting down"
		return
	}

	// Last of all, once nothing can drop it: what is recorded and checked
	// against next time is what people actually saw.
	reply = mind.Casual(reply, mind.CasualStyle{
		Curt:       s.curtWith(grounding, t.item.UserID),
		SlipChance: s.casualSlips,
	}, s.roll())

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
		sp.reason = "Discord refused the message: " + err.Error()
		return
	}

	// The id is what lets a later reply pointing at this message be recognised
	// as a reply to her; see Service.repliesToHer.
	var sentID string
	if sent != nil {
		sentID = sent.ID
	}
	sp.outcome, sp.posted, sp.replyID, sp.took = outcomeAnswered, reply, sentID, time.Since(started)
	switch {
	case t.item.Trigger == mind.TriggerAfterthought:
		s.count(t.item.GuildID, countAfterthought)
	case mind.Volunteered(t.item.Trigger):
		s.count(t.item.GuildID, countVolunteered)
	default:
		s.count(t.item.GuildID, countAnswered)
	}
	spokeAt := time.Now()
	s.conv.Record(t.item.ChannelID, mind.Turn{
		Content:   reply,
		At:        spokeAt,
		FromBot:   true,
		MessageID: sentID,
		To:        t.item.UserID,
	})
	// Persisted because the conversation buffer keeps half an hour and the
	// social drive is measured in hours; see Service.drives.
	if err := s.store.MarkMindSpoke(t.item.GuildID, spokeAt); err != nil {
		s.log.Warn().Err(err).Str("guild_id", t.item.GuildID).Msg("chat_spoke_record_failed")
	}
	s.deferrals.Drop(t.item.ChannelID)
	s.receptionUsed(t.item.ChannelID, t.item.UserID)
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

// send posts the reply, anchored to the message it answers when that helps.
func (s *Service) send(sess *discordgo.Session, t task, content string) (*discordgo.Message, error) {
	return sess.ChannelMessageSendComplex(t.item.ChannelID, s.outgoing(t, content))
}

// outgoing builds the message as Discord will receive it.
//
// Anchored when a bare message would leave people guessing: a held reply, an
// answer to someone who replied to her, or a channel where somebody else has
// spoken since the line she is answering. Not otherwise — see
// mind.NeedsAnchor. An afterthought never is: it follows her own message, and
// quoting the original line again would split one thought across two anchors.
//
// "@Name" she wrote becomes a real mention for anyone in the conversation, but
// only the person she is answering can be notified by it. Letting her ping
// whoever she names would make her an instrument: "tag John and call him a
// butthead" is one message away. Everyone else she names still renders as a
// mention and simply gets no notification; roles, @everyone and @here are
// never parsed at all.
func (s *Service) outgoing(t task, content string) *discordgo.MessageSend {
	recent := s.conv.Recent(t.item.ChannelID)

	resolved, named := mind.ResolveMentions(content, mentionable(recent, t.item))

	// Reaching out has to reach them: if the model did not tag them, the
	// tag goes first. Nobody else is ever tagged this way.
	if t.item.Trigger == mind.TriggerReach && t.item.UserID != "" && !strings.Contains(resolved, "<@"+t.item.UserID+">") {
		resolved = "<@" + t.item.UserID + "> " + resolved
		named = append(named, t.item.UserID)
	}

	allowed := &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	for _, id := range named {
		if id == t.item.UserID {
			allowed.Users = []string{id}
		}
	}

	msg := &discordgo.MessageSend{Content: resolved, AllowedMentions: allowed}

	anchor := t.late || t.item.Trigger == mind.TriggerReply ||
		mind.NeedsAnchor(recent, t.item.MessageID, t.item.UserID)
	if anchor && t.item.Trigger != mind.TriggerAfterthought {
		msg.Reference = &discordgo.MessageReference{
			MessageID: t.item.MessageID,
			ChannelID: t.item.ChannelID,
			GuildID:   t.item.GuildID,
		}
	}
	return msg
}

// mentionable is everyone she could name: the people in the conversation, by
// the names the conversation shows her, and the person she is answering.
func mentionable(turns []mind.Turn, item mind.Deferred) []mind.Person {
	seen := make(map[string]bool)
	var people []mind.Person
	add := func(id, name string) {
		if id == "" || name == "" || seen[id+"\x00"+name] {
			return
		}
		seen[id+"\x00"+name] = true
		people = append(people, mind.Person{ID: id, Name: name})
	}
	add(item.UserID, item.Username)
	for _, turn := range turns {
		if !turn.FromBot {
			add(turn.UserID, turn.Username)
		}
	}
	return people
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
	if t.item.Trigger == mind.TriggerReach {
		g.Reaching, g.Volunteering = t.item.Volunteering, ""
	}
	g.InnerVoice = s.innerVoice
	g.Reception = s.receptionFor(t.item.ChannelID, t.item.UserID, t.item.Username, g.Now)
	if t.item.Trigger == mind.TriggerAfterthought {
		g.Afterthought = mind.AfterthoughtDirective(t.item.FirstLine)
	}
	g.MayDecline = declinable(t)
	if t.item.Closer {
		var facts []mind.Fact
		if p := s.store.GetMindPerson(t.item.GuildID, t.item.UserID); p != nil {
			facts = factsOf(p)
		}
		bring, concrete := mind.SomethingToBring(t.item.Username, facts, g.Remembers, s.roll())
		g.Flat = mind.FlatDirective(t.item.Username, t.item.Content, bring)
		// With something concrete to bring the decision to speak stands: the
		// odds already let most closers go, and a second veto from the model
		// took the rest. With nothing to bring, letting it drop is her call.
		if concrete {
			g.MayDecline = false
		}
	}
	return g
}

// holdSpoken holds an approach for a later attempt and records that it was
// held — or, for something nobody is owed, that it was dropped.
func (s *Service) holdSpoken(t task, sp *spoken, reason string) {
	sp.outcome, sp.reason = outcomeHeld, reason
	if !mind.Owed(t.item.Trigger) {
		sp.outcome = outcomeDropped
	}
	s.hold(t, reason)
}

// curtWith reports whether she is short with the person she is answering.
func (s *Service) curtWith(g mind.Grounding, userID string) bool {
	for _, p := range g.Present {
		if p.UserID == userID {
			return mind.IsCurt(p.Tension, g.Regard)
		}
	}
	return mind.IsCurt(0, g.Regard)
}

// namedProvider is a provider that can say which backend answered. The pool
// is one; a test double need not be.
type namedProvider interface {
	GenerateNamed(ctx context.Context, messages []ai.Message) (string, string, error)
}

// generate asks the provider for a reply and reports which backend gave it,
// when the provider can say.
func (s *Service) generate(ctx context.Context, messages []ai.Message) (string, string, error) {
	if named, ok := s.provider.(namedProvider); ok {
		return named.GenerateNamed(ctx, messages)
	}
	reply, err := s.provider.Generate(ctx, messages)
	return reply, "", err
}

// declinable reports whether she may still answer SKIP to this approach:
// something she was not directly asked, or a message that closed the topic.
func declinable(t task) bool {
	return !t.late && (mind.MayDecline(t.item.Trigger) || t.item.Closer)
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
			who.Tension = mind.TensionNow(known.Tension, known.TensionAt, time.Now())
			who.Closeness = mind.ClosenessNow(known.Closeness, known.ClosenessAt, time.Now())
			who.Facts = factsOf(known)
			who.Impression = known.Impression
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
