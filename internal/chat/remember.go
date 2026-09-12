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

		turns := s.conv.All(channelID)
		if len(turns) < worthRemembering || !mind.Settled(turns, now, settleFor) {
			continue
		}
		if s.alreadyRemembered(guildID, channelID, turns) {
			continue
		}

		s.remembered(ctx, guildID, channelID, turns)
	}
}

// alreadyRemembered reports whether the newest turn is already covered by a
// memory.
//
// Derived from the stored memories rather than from a marker held in memory,
// so a restart does not cause the same conversation to be remembered twice —
// which would cost a second backend call to produce a duplicate.
func (s *Service) alreadyRemembered(guildID, channelID string, turns []mind.Turn) bool {
	newest := turns[len(turns)-1].At

	for _, m := range s.store.MindMemories(guildID, channelID) {
		if !m.At.Before(newest) {
			return true
		}
	}
	return false
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

	gist, detail, ok := mind.ParseSummary(reply)
	if !ok {
		s.log.Debug().
			Str("guild_id", guildID).
			Str("channel_id", channelID).
			Str("reply", trimForLog(reply)).
			Msg("chat_memory_unreadable")
		return
	}

	memory := storage.MindMemory{
		GuildID:   guildID,
		ChannelID: channelID,
		At:        turns[len(turns)-1].At,
		Gist:      gist,
		Detail:    detail,
		Weight:    mind.WeighMoment(turns),
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
		Str("gist", gist).
		Msg("chat_remembered")
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
