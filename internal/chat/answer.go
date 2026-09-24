package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Settling: she reads a burst once its author has stopped typing, rather
// than answering its first line while the rest is still arriving.
const (
	settleQuiet = 3 * time.Second
	settleMax   = 12 * time.Second
)

// Journal outcomes, as /chat why shows them.
const (
	outcomeAnswered = "answered"
	outcomeReacted  = "reacted"
	outcomeSilent   = "stayed quiet"
	outcomeDropped  = "dropped"
	outcomeHeld     = "held for later"
	outcomeAway     = "she was away"
	outcomeAsleep   = "she was asleep"
)

// handle takes one moment from arrival to whatever she does about it.
func (s *Service) handle(ctx context.Context, t task) {
	sess := s.session()
	if sess == nil {
		s.hold(t, "no gateway session")
		return
	}
	if t.item.Trigger == mind.TriggerLeave {
		s.leave(ctx, sess, t)
		return
	}
	if !t.catchUp && !s.online() {
		// She left between this arriving and a worker reaching it.
		s.miss(t.item)
		return
	}
	if !t.late && !t.catchUp && !s.settle(ctx, t.item) {
		return
	}
	s.backfill(sess, t.item.ChannelID)

	genCtx, cancel := context.WithTimeout(ctx, s.generateTimeout)
	defer cancel()

	scene := s.scene(sess, t)
	known, err := s.mind.Know(scene)
	if err != nil {
		// Her memory failing to read is not a reason to go quiet; she
		// answers from the conversation alone.
		s.log.Warn().Err(err).Str("guild_id", scene.GuildID).Msg("chat_memory_read_failed")
	}

	entry := storage.MindJournal{
		GuildID: scene.GuildID, ChannelID: scene.ChannelID, At: scene.Now,
		MessageID: t.item.MessageID, UserID: t.item.UserID, Username: t.item.Username,
		Excerpt: excerpt(t.item.Content, journalExcerpt), Trigger: string(t.item.Trigger),
	}
	defer func() { s.journal(entry) }()

	var a mind.Appraisal
	if t.item.Considered != nil {
		a = *t.item.Considered
	} else {
		a, err = s.mind.Consider(genCtx, scene, known)
		if err != nil && !errors.Is(err, mind.ErrUnreadable) {
			if ctx.Err() == nil {
				s.log.Warn().Err(err).Str("guild_id", scene.GuildID).Msg("chat_consider_failed")
				entry.Outcome, entry.Reason = s.hold(t, "no backend would think it through")
			}
			return
		}
		if err != nil {
			// Answered, but not in the shape asked for. Asking again tends
			// to get the same shape back, so she answers plainly if she was
			// plainly asked, and otherwise lets it go.
			s.log.Warn().Err(err).Str("guild_id", scene.GuildID).Msg("chat_appraisal_unreadable")
			a = mind.Appraisal{Act: mind.ActIgnore, Backend: a.Backend}
			if mind.Direct(t.item.Trigger) || t.item.Trigger == mind.TriggerFollowUp {
				a.Act = mind.ActReply
			}
		}
		if t.catchUp {
			// Silence about something from hours ago reads as the moment
			// having passed, not as a broken bot: not overruled.
		} else if reason := s.overrule(t.item, &a); reason != "" {
			entry.Reason = reason
		}
		if t.item.ReactOnly && a.Act == mind.ActReply {
			// A room where she only answers: she may react to what she
			// overheard, never speak up. A rail, not a reading.
			a.Act = mind.ActIgnore
			entry.Reason = "she only reacts where she was not asked"
		}
		if err := s.mind.Absorb(scene, a); err != nil {
			s.log.Warn().Err(err).Str("guild_id", scene.GuildID).Msg("chat_memory_write_failed")
		}
		if a.BackOff {
			s.withdrawConsent(scene.GuildID, t.item.UserID)
		}
		if a.Drop {
			// Their asking is the end of it: one short thing, and she
			// leaves it. Whatever she had meant to get across goes with
			// it — the ask is the thing they asked her to stop making.
			// The code keeps her to that; see stop.go.
			s.dropIt(scene.GuildID, t.item.UserID, s.now())
			a.Intent, a.Then = dropIntent, ""
			entry.Intent = a.Intent
			entry.Reason = "they asked her to drop it"
		} else if a.Act == mind.ActReply && s.pressing(scene.GuildID, scene.ChannelID, t.item.UserID, a.Intent, s.now()) {
			// The same ask, again. She has made her point; making it a
			// fourth time is what an evening of pressing is made of.
			a.Act, a.Then = mind.ActIgnore, ""
			entry.Reason = "she has put that to them enough times"
		}
	}
	if a.Look != "" {
		if caught, why := s.lookAt(genCtx, sess, scene, a.Look); why != "" {
			entry.Reason = why
		} else if caught != "" {
			// What she just saw is hers now: read her memory again so the
			// answer has it.
			if fresh, err := s.mind.Know(scene); err == nil {
				known = fresh
			}
		}
	}
	s.drain(scene)
	s.applyEnergy(scene.GuildID, t.item.UserID, a.Energy, s.now())
	entry.Read, entry.Feel, entry.Toward, entry.Mood = a.Read, a.Feel, a.Toward, a.Mood
	entry.Situation = string(a.Situation)
	entry.Act, entry.Intent, entry.Backend = string(a.Act), a.Intent, a.Backend

	switch a.Act {
	case mind.ActIgnore:
		s.letGo(scene, t.item, a)
		entry.Outcome = outcomeSilent
		s.log.Info().
			Str("guild_id", scene.GuildID).
			Str("channel_id", scene.ChannelID).
			Str("trigger", string(t.item.Trigger)).
			Msg("chat_approach_ignored")
		return
	case mind.ActReact:
		s.setIgnored(t.item, false)
		if err := sess.MessageReactionAdd(scene.ChannelID, t.item.MessageID, strings.TrimSpace(a.Emoji)); err != nil {
			s.log.Warn().Err(err).Str("channel_id", scene.ChannelID).Msg("chat_react_failed")
			entry.Outcome, entry.Reason = outcomeDropped, "Discord refused the reaction"
			return
		}
		s.letGo(scene, t.item, a)
		entry.Outcome = outcomeReacted
		if mind.Unprompted(t.item.Trigger) {
			s.reacted(scene.GuildID, scene.Now)
		}
		return
	}

	s.setIgnored(t.item, false)
	if !t.late {
		if err := sess.ChannelTyping(scene.ChannelID); err != nil {
			s.log.Debug().Err(err).Str("channel_id", scene.ChannelID).Msg("chat_typing_failed")
		}
	}
	started := s.now()
	why := ""
	if scene.Thread != nil {
		why = "you meant to follow up with them: " + scene.Thread.Text
	}
	reply, backend, err := s.speak(genCtx, scene, known, a, why)
	if backend != "" {
		entry.Backend = backend
	}
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.log.Warn().Err(err).Str("guild_id", scene.GuildID).Msg("chat_speak_failed")
		if errors.Is(err, mind.ErrRepeat) {
			// A retry later would build the same prompt and get the same
			// line; silence is better than a loop.
			entry.Outcome, entry.Reason = outcomeDropped, "she kept repeating herself"
			return
		}
		held := t
		held.item.Considered = &a
		entry.Outcome, entry.Reason = s.hold(held, "no usable words came back: "+err.Error())
		return
	}

	sent, err := s.deliver(ctx, sess, scene, reply, started)
	if err != nil {
		s.log.Warn().Err(err).Str("channel_id", scene.ChannelID).Msg("chat_send_failed")
		entry.Outcome, entry.Reason = outcomeDropped, "Discord refused the message: "+err.Error()
		return
	}
	s.deferrals.Drop(scene.ChannelID)
	if err := s.mind.Said(scene, a, sent.text, why, sent.id); err != nil {
		s.log.Warn().Err(err).Str("guild_id", scene.GuildID).Msg("chat_memory_write_failed")
	}
	s.afterUnprompted(scene, sent)
	entry.Outcome, entry.Posted, entry.ReplyID = outcomeAnswered, excerpt(sent.text, journalReply), sent.id
	entry.Took = s.now().Sub(started)
	s.secondThought(scene, a, sent)
	s.log.Info().
		Str("guild_id", scene.GuildID).
		Str("channel_id", scene.ChannelID).
		Str("trigger", string(t.item.Trigger)).
		Bool("late", t.late).
		Int("chars", len(sent.text)).
		Msg("chat_replied")
}

// overrule keeps the one promise about silence the model cannot be trusted
// with: she never ignores the same person's direct approach twice running.
//
// From outside, a deliberate silence and a broken bot look identical. One is
// a person choosing; two in a row is a fault as far as anyone can tell, and
// in production it read exactly that way. It returns why it stepped in, or
// "" when it did not.
func (s *Service) overrule(item mind.Deferred, a *mind.Appraisal) string {
	if !mind.Direct(item.Trigger) || a.Act != mind.ActIgnore {
		if mind.Direct(item.Trigger) {
			s.setIgnored(item, false)
		}
		return ""
	}
	s.ignoredMu.Lock()
	twice := s.ignored[answerKey(item.GuildID, item.ChannelID, item.UserID)]
	s.ignoredMu.Unlock()
	if !twice {
		s.setIgnored(item, true)
		return ""
	}
	a.Act = mind.ActReply
	if a.Intent == "" {
		a.Intent = "answer them, briefly and in your own way — you let their last one go"
	}
	return "she let their last one go too; a second silence in a row reads as broken"
}

func (s *Service) setIgnored(item mind.Deferred, v bool) {
	if !mind.Direct(item.Trigger) {
		return
	}
	key := answerKey(item.GuildID, item.ChannelID, item.UserID)
	s.ignoredMu.Lock()
	if v {
		s.ignored[key] = true
	} else {
		delete(s.ignored, key)
	}
	s.ignoredMu.Unlock()
}

// letGo records a moment she did not answer in words.
func (s *Service) letGo(scene mind.Scene, item mind.Deferred, a mind.Appraisal) {
	if err := s.mind.LetGo(scene, a); err != nil {
		s.log.Warn().Err(err).Str("guild_id", item.GuildID).Msg("chat_memory_write_failed")
	}
}

// settle waits for the person to stop typing, up to a limit, and reports
// whether to go on. A burst is one approach: answering its first line while
// the second is being typed is how a bot talks over people.
//
// Bounded by a count of waits as well as by the clock, so a clock that does
// not move — a test's, or one stepped back by the host — cannot hold a worker
// forever.
func (s *Service) settle(ctx context.Context, item mind.Deferred) bool {
	if s.settleQuiet <= 0 {
		return true
	}
	// Tired, she takes longer to get round to reading.
	quiet := time.Duration(float64(s.settleQuiet) * s.slowness())
	for waits := 0; waits < int(settleMax/quiet)+1; waits++ {
		last := item.FormedAt
		for _, t := range s.conv.Recent(item.ChannelID) {
			if t.UserID == item.UserID && t.At.After(last) {
				last = t.At
			}
		}
		wait := quiet - s.now().Sub(last)
		if wait <= 0 {
			return true
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
	return true
}

// hold puts an answer back for a later attempt, or gives up on something she
// started — which belongs to its moment. It returns the journal outcome.
func (s *Service) hold(t task, reason string) (string, string) {
	if !mind.Owed(t.item.Trigger) {
		return outcomeDropped, reason
	}
	if s.deferrals.Hold(t.item, s.now()) {
		return outcomeHeld, reason
	}
	s.log.Info().
		Str("guild_id", t.item.GuildID).
		Str("channel_id", t.item.ChannelID).
		Int("attempts", t.item.Attempts).
		Str("reason", reason).
		Msg("chat_approach_abandoned")
	return outcomeDropped, reason + "; gave up"
}

// scene assembles what the moment is, from the session's cache: this runs per
// moment, and fetching a guild and a channel each time would spend rate limit
// on facts the gateway already sent.
func (s *Service) scene(sess *discordgo.Session, t task) mind.Scene {
	now := s.now().In(s.location)
	sc := mind.Scene{
		GuildID:   t.item.GuildID,
		ChannelID: t.item.ChannelID,
		Brief:     s.store.GetChatBrief(t.item.GuildID),
		SelfName:  s.DisplayName(sess, t.item.GuildID),
		Now:       now,
		Turns:     s.conv.Recent(t.item.ChannelID),
		Trigger:   t.item.Trigger,
		UserID:    t.item.UserID,
		Username:  t.item.Username,
		MessageID: t.item.MessageID,
	}
	if t.late {
		sc.Late = t.item.Age(now)
	}
	sc.Thread = t.item.Thread
	if guild, err := sess.State.Guild(t.item.GuildID); err == nil && guild != nil {
		sc.GuildName = guild.Name
	}
	if channel, err := sess.State.Channel(t.item.ChannelID); err == nil && channel != nil {
		sc.ChannelName = channel.Name
		sc.ChannelTopic = channel.Topic
	}
	sc.Roles = s.roleNotes(sess, sc.GuildID, sc)
	sc.Reactions = s.reactionsIn(sc.ChannelID, sc.Turns)
	sc.QuietFor = s.quietFor(sc.GuildID, now)
	sc.ReactOnly = t.item.ReactOnly
	sc.Crowd = t.item.Crowd
	sc.Reads = s.readNames(sess, t.item.GuildID)
	sc.Dropped = s.droppedAt(t.item.GuildID, t.item.UserID, now)
	s.bodyScene(&sc, now)
	return sc
}

// roleNotes is what an administrator said about the roles of the people in a
// scene, by user id. See /chat role.
func (s *Service) roleNotes(sess *discordgo.Session, guildID string, sc mind.Scene) map[string]string {
	biases := s.store.ChatRoleBiases(guildID)
	if len(biases) == 0 || sess == nil || sess.State == nil {
		return nil
	}
	out := make(map[string]string)
	for _, id := range sceneIDs(sc) {
		member, err := sess.State.Member(guildID, id)
		if err != nil || member == nil {
			continue
		}
		var notes []string
		for _, roleID := range member.Roles {
			if note := strings.TrimSpace(biases[roleID].Note); note != "" {
				notes = append(notes, note)
			}
		}
		if len(notes) > 0 {
			out[id] = strings.Join(notes, "; ")
		}
	}
	return out
}

// sceneIDs is everyone in a scene.
func sceneIDs(sc mind.Scene) []string {
	seen := map[string]bool{"": true}
	var ids []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(sc.UserID)
	for _, t := range sc.Turns {
		if !t.FromBot {
			add(t.UserID)
		}
	}
	return ids
}
