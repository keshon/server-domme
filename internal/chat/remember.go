package chat

import (
	"context"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Memory writing cadence.
const (
	// rememberInterval is how often channels are considered for a memory. The
	// sweep itself is free; what it guards is at most one backend call per
	// channel per pass, and only for channels where something happened.
	rememberInterval = 5 * time.Minute
	// settleFor is how long a conversation has to have been quiet before it is
	// remembered as one thing. Below this it is still in progress, and
	// summarising it produces a memory of half an argument followed by a
	// second memory of the other half.
	settleFor = 6 * time.Minute
	// worthRemembering is the fewest turns that make an exchange worth a
	// backend call. Two people saying good morning is not a memory.
	worthRemembering = 6
	// rememberTimeout bounds one summary call. Generous: nothing waits on it,
	// and a summary that takes a minute costs nobody anything.
	rememberTimeout = 2 * time.Minute
	// sessionGap is the silence that ends one conversation and starts the
	// next. Longer than settleFor: a six-minute pause decides that talk has
	// stopped, but people pick a thread back up after ten.
	sessionGap = 30 * time.Minute
)

// rememberLoop writes memories for conversations that have finished.
//
// Runs on its own goroutine under the service's context, and never on the path
// of a reply. Everything here is best-effort by design: a failed summary means
// one conversation goes unremembered, which nobody can see, whereas a summary
// that blocked or delayed an answer would be obvious to everyone in the
// channel.
func (s *Service) rememberLoop(ctx context.Context) {
	ticker := time.NewTicker(rememberInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rememberSettled(ctx)
		}
	}
}

// rememberSettled walks the live channels and remembers the ones that have
// gone quiet.
func (s *Service) rememberSettled(ctx context.Context) {
	s.catchUp()
	now := time.Now()

	for _, channelID := range s.conv.Channels() {
		if ctx.Err() != nil {
			return
		}

		guildID := s.guildOf(channelID)
		if guildID == "" {
			continue
		}
		// Checked here and not only in Observe. Summarising sends the channel's
		// contents to a third-party relay, which is the exact thing the opt-in
		// governs, and the conversation outlives the opt-in: silencing a
		// channel leaves its turns in the buffer, where this sweep would still
		// find them.
		if !s.store.IsChatChannel(guildID, channelID) {
			continue
		}

		// Only the latest conversation, and only what no memory covers yet.
		// The buffer can hold days of a quiet channel once history has been
		// read back, and summarising all of it folds last week's argument and
		// this morning's greeting into one memory — some of it for the
		// second time.
		turns := mind.LastSession(s.unremembered(guildID, channelID, s.conv.All(channelID)), sessionGap)
		if len(turns) < worthRemembering || !mind.Settled(turns, now, settleFor) {
			continue
		}

		s.remembered(ctx, guildID, channelID, turns)
	}
}

// catchUp rereads every opted-in channel the process has not seen yet.
//
// The conversation buffer is in memory, and without this a redeploy threw
// away whatever had not yet been remembered: the sweep only looks at channels
// in the buffer, and a channel only came back into it when she next spoke
// there. Every deploy that landed inside a conversation, or in the six minutes
// after it, cost her that conversation. Reading the history back from Discord
// puts it where the sweep can see it, and unremembered keeps anything
// that was stored before the restart from being summarised twice.
//
// One REST call per channel per process, because backfill marks a channel
// done whether or not the read succeeds.
func (s *Service) catchUp() {
	sess := s.session()
	if sess == nil {
		return
	}
	for channelID, guildID := range s.store.AllChatChannels() {
		if !s.conv.NeedsSeed(channelID) {
			continue
		}
		s.noteGuild(guildID, channelID)
		s.backfill(sess, channelID)
	}
}

// unremembered drops the turns a stored memory already covers.
//
// Derived from the stored memories rather than from a marker held in memory,
// so a restart does not cause the same conversation to be remembered twice —
// which would cost a second backend call to produce a duplicate. A memory is
// stamped with the time of the last turn it covers, so everything up to and
// including that moment is done.
func (s *Service) unremembered(guildID, channelID string, turns []mind.Turn) []mind.Turn {
	var covered time.Time
	for _, m := range s.store.MindMemories(guildID, channelID) {
		if m.At.After(covered) {
			covered = m.At
		}
	}
	if covered.IsZero() {
		return turns
	}
	for i, t := range turns {
		if t.At.After(covered) {
			return turns[i:]
		}
	}
	return nil
}

// remembered asks a backend to summarise a finished conversation and stores
// the result.
func (s *Service) remembered(ctx context.Context, guildID, channelID string, turns []mind.Turn) {
	callCtx, cancel := context.WithTimeout(ctx, rememberTimeout)
	reply, err := s.provider.Generate(callCtx, mind.SummaryPrompt(turns))
	cancel()

	if err != nil {
		// Not held and not retried. A deferral exists so a person gets their
		// answer late rather than never; nobody is waiting on a memory, and
		// the conversation will still be there next sweep if it is worth
		// having.
		s.log.Debug().
			Err(err).
			Str("guild_id", guildID).
			Str("channel_id", channelID).
			Msg("chat_memory_generate_failed")
		return
	}

	gist, detail, tone, ok := mind.ParseSummary(reply)
	if !ok {
		s.log.Debug().
			Str("guild_id", guildID).
			Str("channel_id", channelID).
			Str("reply", trimForLog(reply)).
			Msg("chat_memory_unreadable")
		return
	}

	at := turns[len(turns)-1].At
	memory := storage.MindMemory{
		GuildID:   guildID,
		ChannelID: channelID,
		At:        at,
		Gist:      gist,
		Detail:    detail,
		Weight:    mind.WeighTone(mind.WeighMoment(turns), tone),
		People:    mind.Participants(turns),
	}
	if err := s.store.AddMindMemory(memory); err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_memory_store_failed")
		return
	}

	s.log.Info().
		Str("guild_id", guildID).
		Str("channel_id", channelID).
		Int("turns", len(turns)).
		Float64("weight", memory.Weight).
		Str("tone", string(tone)).
		Str("gist", gist).
		Msg("chat_remembered")

	s.feelConversation(guildID, memory.People, tone, at)
	s.notePeople(ctx, guildID, turns, at)
}

// maxLoggedReply caps an unreadable summary in the log. A backend that answers
// with an HTML error page should not put the whole of it in the journal.
const maxLoggedReply = 200

func trimForLog(s string) string {
	if len(s) > maxLoggedReply {
		return s[:maxLoggedReply] + "…"
	}
	return s
}
