package chat

import (
	"fmt"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// awaiting is something she started, waiting to see how it lands.
type awaiting struct {
	guildID, channelID string
	// userID is who it was aimed at, or empty for something she said to the
	// room, which anyone can take up.
	userID  string
	trigger mind.Trigger
	at      time.Time
	journal uint64
	// expected is what she expected, taken when she spoke: her welcome with
	// them, or neutral for the room.
	expected float64
}

// awaitPayoff starts watching how something she started is received.
// Reaching out is not watched here: it learns from its own answered and
// unanswered counts, which span hours rather than minutes.
func (s *Service) awaitPayoff(t task, at time.Time) {
	switch t.item.Trigger {
	case mind.TriggerReturn, mind.TriggerRecall, mind.TriggerAfterthought:
	default:
		return
	}
	// Neutral is what an unknown bond reads as: a room has no welcome of its
	// own to have learned.
	_, _, neutral := mind.Bond{}.Now(at)
	a := awaiting{
		guildID: t.item.GuildID, channelID: t.item.ChannelID,
		trigger: t.item.Trigger, at: at, journal: t.item.Journal,
		expected: neutral,
	}
	// A remark about a subject is to the room; a greeting or a second
	// thought is to one person, and it is their reception that counts.
	if t.item.Trigger != mind.TriggerRecall {
		a.userID = t.item.UserID
		a.expected = s.welcomeOf(t.item.GuildID, t.item.UserID, at)
	}
	s.payoffMu.Lock()
	s.payoffs[t.item.ChannelID] = append(s.payoffs[t.item.ChannelID], a)
	s.payoffMu.Unlock()
}

// settlePayoffs resolves what she is waiting on in a channel when someone
// speaks to her there: taken up, laughed at, or panned.
func (s *Service) settlePayoffs(channelID, userID, content string, now time.Time) {
	s.payoffMu.Lock()
	var settled, kept []awaiting
	for _, a := range s.payoffs[channelID] {
		if now.Sub(a.at) <= mind.PayoffWindow && (a.userID == "" || a.userID == userID) {
			settled = append(settled, a)
		} else {
			kept = append(kept, a)
		}
	}
	s.payoffs[channelID] = kept
	s.payoffMu.Unlock()

	outcome := mind.PayoffOf(content)
	for _, a := range settled {
		s.paidOff(a, outcome, now)
	}
}

// expirePayoffs settles as ignored whatever has waited out its window.
func (s *Service) expirePayoffs(now time.Time) {
	s.payoffMu.Lock()
	var expired []awaiting
	for channelID, list := range s.payoffs {
		var kept []awaiting
		for _, a := range list {
			if now.Sub(a.at) > mind.PayoffWindow {
				expired = append(expired, a)
			} else {
				kept = append(kept, a)
			}
		}
		if len(kept) == 0 {
			delete(s.payoffs, channelID)
		} else {
			s.payoffs[channelID] = kept
		}
	}
	s.payoffMu.Unlock()

	for _, a := range expired {
		s.paidOff(a, mind.PayoffIgnored, now)
	}
}

// paidOff is what an outcome does to her: the surprise moves her mood, and
// the person's reception teaches her how welcome she is with them.
func (s *Service) paidOff(a awaiting, outcome mind.Payoff, now time.Time) {
	surprise := mind.Surprise(outcome, a.expected)
	s.moveMood(a.guildID, mind.SurpriseMood(surprise), now)
	if a.userID != "" {
		s.appraise(a.guildID, a.userID, mind.PayoffEvent(outcome), now)
	}

	note := fmt.Sprintf("%s — expected %.2f, surprise %+.2f", outcome, a.expected, surprise)
	s.journalUpdate(a.guildID, a.journal, func(j *storage.MindJournal) { j.Payoff = note })
	s.log.Info().
		Str("guild_id", a.guildID).
		Str("channel_id", a.channelID).
		Str("trigger", string(a.trigger)).
		Str("payoff", string(outcome)).
		Float64("surprise", surprise).
		Msg("chat_paid_off")
}

// welcomeOf is how welcome she has learned she is with someone, decayed to
// now; neutral for someone she has never learned anything about.
func (s *Service) welcomeOf(guildID, userID string, now time.Time) float64 {
	_, _, welcome := bondOf(s.store.GetMindPerson(guildID, userID)).Now(now)
	return welcome
}

// markContact records that someone spoke to her, which is what satisfies
// her need for company.
func (s *Service) markContact(guildID string, now time.Time) {
	err := s.store.UpdateMindGuild(guildID, func(g *storage.MindGuild) { g.LastContactAt = now })
	if err != nil {
		s.log.Debug().Err(err).Str("guild_id", guildID).Msg("chat_contact_record_failed")
	}
}
