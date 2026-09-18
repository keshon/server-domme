package chat

import (
	"context"
	"time"

	"github.com/bwmarrin/discordgo"
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

// afterthoughtTyping is how long "is typing" shows before an afterthought
// lands. Long enough to register as someone typing a short line, short enough
// that the second thought still follows the first.
const afterthoughtTyping = 1500 * time.Millisecond

// typeBriefly shows her typing just before an afterthought is sent, and
// reports whether it should still go out.
//
// Only here, once it is certain to be sent. Shown before generating, as an
// answer's typing is, it announced a message the model then declined to
// write, or one dropped because the person had answered meanwhile — typing
// that stops with nothing posted, which reads as her writing something and
// deleting it. The last word is checked again after the pause for the same
// reason.
func (s *Service) typeBriefly(ctx context.Context, sess *discordgo.Session, t task) bool {
	if err := sess.ChannelTyping(t.item.ChannelID); err != nil {
		s.log.Debug().Err(err).Str("channel_id", t.item.ChannelID).Msg("chat_typing_failed")
	}
	timer := time.NewTimer(afterthoughtTyping)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
	}
	if !mind.LastWord(s.conv.Recent(t.item.ChannelID), t.after) {
		s.log.Debug().Str("channel_id", t.item.ChannelID).Msg("chat_afterthought_overtaken")
		return false
	}
	return true
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
