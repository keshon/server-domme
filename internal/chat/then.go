package chat

import (
	"context"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Second thoughts. When she decides what to say, she may also decide there is
// one more thing she will want to add a little later — a question her own
// reply leaves her curious about, a thought on its heels. It is born with the
// reply, in the same appraisal, and waits here for its moment. See
// mind.Appraisal.Then.
//
// v1 had afterthoughts too, and there the code decided by a roll whether to
// ask the model for one; the model then had to invent something to say. Here
// she only adds something she already had in mind, and the code's part is to
// check that the moment is still there.
const (
	// thoughtTick is how often waiting thoughts are looked at.
	thoughtTick = 2 * time.Second
	// thoughtsPerHour is how many second thoughts she sends in a guild in an
	// hour. More, and the double-text becomes a rhythm.
	thoughtsPerHour = 2
)

// pendingThought is a second thought waiting to be sent.
type pendingThought struct {
	scene mind.Scene
	then  string
	// after is her message it follows. It is only sent while that message
	// is still the last word in the channel.
	after string
	due   time.Time
}

// secondThought queues what she means to add after a reply, if anything.
func (s *Service) secondThought(sc mind.Scene, a mind.Appraisal, sent sentMessage) {
	if a.Then == "" || sent.id == "" || sc.Trigger == mind.TriggerThen || sc.Late > 0 {
		return
	}
	s.thoughtMu.Lock()
	defer s.thoughtMu.Unlock()
	s.thoughts = append(s.thoughts, pendingThought{
		scene: sc, then: a.Then, after: sent.id, due: s.now().Add(a.ThenAfter),
	})
}

func (s *Service) thoughtLoop(ctx context.Context) {
	ticker := time.NewTicker(thoughtTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dueThoughts(ctx)
		}
	}
}

// dueThoughts sends every second thought whose moment has come, and drops
// those whose moment has passed.
func (s *Service) dueThoughts(ctx context.Context) {
	now := s.now()
	s.thoughtMu.Lock()
	var due, waiting []pendingThought
	for _, t := range s.thoughts {
		if t.due.After(now) {
			waiting = append(waiting, t)
		} else {
			due = append(due, t)
		}
	}
	s.thoughts = waiting
	s.thoughtMu.Unlock()

	for _, t := range due {
		if ctx.Err() != nil {
			return
		}
		s.sendThought(ctx, t)
	}
}

// sendThought sends one second thought, if its moment is still there.
//
// Only while her message is still the last word. If the person has answered,
// the thought belongs to a conversation that has moved on, and a line from
// her arriving after their reply is the tell this exists to avoid. If anyone
// else has spoken, it would land in the middle of their exchange.
func (s *Service) sendThought(ctx context.Context, t pendingThought) {
	if !mind.LastWord(s.conv.Recent(t.scene.ChannelID), t.after) {
		s.log.Debug().Str("channel_id", t.scene.ChannelID).Msg("chat_thought_overtaken")
		return
	}
	if !s.mayThink(t.scene.GuildID) {
		return
	}
	sess := s.session()
	if sess == nil {
		return
	}

	sc := t.scene
	sc.Trigger, sc.Now, sc.MessageID = mind.TriggerThen, s.now().In(s.location), ""
	sc.Turns = s.conv.Recent(sc.ChannelID)
	a := mind.Appraisal{Act: mind.ActReply, Intent: t.then}
	entry := storage.MindJournal{
		GuildID: sc.GuildID, ChannelID: sc.ChannelID, At: sc.Now,
		UserID: sc.UserID, Username: sc.Username, Trigger: string(sc.Trigger),
		Act: string(mind.ActReply), Intent: t.then,
	}
	defer func() { s.journal(entry) }()

	genCtx, cancel := context.WithTimeout(ctx, s.generateTimeout)
	defer cancel()
	known, err := s.mind.Know(sc)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_memory_read_failed")
	}
	started := s.now()
	reply, backend, err := s.speak(genCtx, sc, known, a, "")
	entry.Backend = backend
	if err != nil {
		s.log.Info().Err(err).Str("channel_id", sc.ChannelID).Msg("chat_thought_dropped")
		entry.Outcome, entry.Reason = outcomeDropped, err.Error()
		return
	}
	// Checked again: the person may have answered while she was writing.
	if !mind.LastWord(s.conv.Recent(sc.ChannelID), t.after) {
		entry.Outcome, entry.Reason = outcomeDropped, "they answered while she was writing it"
		return
	}
	if err := sess.ChannelTyping(sc.ChannelID); err != nil {
		s.log.Debug().Err(err).Str("channel_id", sc.ChannelID).Msg("chat_typing_failed")
	}
	sent, err := s.deliver(genCtx, sess, sc, reply, started)
	if err != nil {
		entry.Outcome, entry.Reason = outcomeDropped, "Discord refused the message: "+err.Error()
		return
	}
	s.thought(sc.GuildID)
	entry.Outcome, entry.Posted, entry.ReplyID = outcomeAnswered, excerpt(sent.text, journalReply), sent.id
	if err := s.mind.Said(sc, a, sent.text, "", sent.id); err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_memory_write_failed")
	}
	s.log.Info().Str("guild_id", sc.GuildID).Str("channel_id", sc.ChannelID).Msg("chat_thought_sent")
}

// mayThink reports whether the hour's second thoughts in a guild allow one
// more.
func (s *Service) mayThink(guildID string) bool {
	s.thoughtMu.Lock()
	defer s.thoughtMu.Unlock()
	var recent []time.Time
	for _, at := range s.thoughtTimes[guildID] {
		if s.now().Sub(at) < time.Hour {
			recent = append(recent, at)
		}
	}
	s.thoughtTimes[guildID] = recent
	return len(recent) < thoughtsPerHour
}

// thought counts one second thought sent.
func (s *Service) thought(guildID string) {
	s.thoughtMu.Lock()
	defer s.thoughtMu.Unlock()
	s.thoughtTimes[guildID] = append(s.thoughtTimes[guildID], s.now())
}
