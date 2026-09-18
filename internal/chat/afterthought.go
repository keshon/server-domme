package chat

import (
	"context"
	"time"

	"github.com/keshon/server-domme/internal/mind"
)

// considerAfterthought decides, right after she has spoken, whether a second
// message may follow, and if so schedules it.
//
// The decision is made here, deterministically; whether anything is actually
// said is left to the model, which is allowed to decline. See
// mind.MayAddAfterthought and mind.AfterthoughtDirective.
func (s *Service) considerAfterthought(ctx context.Context, t task, g mind.Grounding, reply, sentID string, at time.Time) {
	if sentID == "" {
		return
	}

	var irritation float64
	for _, p := range g.Present {
		if p.UserID == t.item.UserID {
			irritation = p.Irritation
		}
	}

	s.afterthoughtMu.Lock()
	last := s.afterthoughts[t.item.ChannelID]
	s.afterthoughtMu.Unlock()

	allowed := mind.MayAddAfterthought(mind.Afterthought{
		Trigger:    t.item.Trigger,
		Late:       t.late,
		Reply:      reply,
		Now:        at,
		Last:       last,
		Turns:      s.conv.Recent(t.item.ChannelID),
		UserID:     t.item.UserID,
		Drives:     g.Drives,
		Irritation: irritation,
		Regard:     g.Regard,
	}, s.roll())
	if !allowed {
		return
	}

	// Stamped when allowed rather than when sent, so a declined or failed one
	// still spends the cooldown. The alternative is asking again after every
	// short reply until the model finally says something.
	s.afterthoughtMu.Lock()
	s.afterthoughts[t.item.ChannelID] = at
	s.afterthoughtMu.Unlock()

	item := t.item
	item.Trigger = mind.TriggerAfterthought
	item.FirstLine = reply
	item.Volunteering = ""
	item.Attempts = 0
	followUp := task{item: item, after: sentID}

	delay := mind.AfterthoughtDelay(s.roll())
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.enqueueAfterthought(followUp)
	}()
}

// enqueueAfterthought hands a scheduled afterthought to a worker, unless the
// conversation has moved on while she paused.
func (s *Service) enqueueAfterthought(t task) {
	if !mind.LastWord(s.conv.Recent(t.item.ChannelID), t.after) {
		s.log.Debug().Str("channel_id", t.item.ChannelID).Msg("chat_afterthought_overtaken")
		return
	}
	select {
	case s.work <- t:
	default:
		s.log.Debug().Str("channel_id", t.item.ChannelID).Msg("chat_afterthought_dropped_busy")
	}
}

// afterthoughtStands reports whether a generated afterthought should be sent.
//
// Checked after generation, which takes seconds: the person may have answered
// in the meantime, and a second line landing after their reply is worse than
// none.
func (s *Service) afterthoughtStands(t task, reply string) bool {
	reason := ""
	switch {
	case mind.IsSkip(reply):
		reason = "declined"
	case mind.SameLine(reply, t.item.FirstLine):
		reason = "repeated"
	case !mind.LastWord(s.conv.Recent(t.item.ChannelID), t.after):
		reason = "overtaken"
	}
	if reason == "" {
		return true
	}
	s.log.Info().
		Str("guild_id", t.item.GuildID).
		Str("channel_id", t.item.ChannelID).
		Str("reason", reason).
		Msg("chat_afterthought_withheld")
	return false
}
