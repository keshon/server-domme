package chat

import (
	"fmt"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// fatigue is how much she has put herself forward lately in a guild, decayed
// to now. See mind.Fatigue.
func (s *Service) fatigue(guildID string, now time.Time) float64 {
	g := s.store.GetMindGuild(guildID)
	if g == nil {
		return 0
	}
	return mind.Fatigue{Level: g.Initiative, At: g.InitiativeAt}.Now(now)
}

// spendInitiative records that she took the initiative, which is what makes
// the next time less likely.
//
// Spent when decided rather than when sent, like the daily counts before it:
// one that then fails to generate or is declined still tires her, which errs
// towards her saying less — the right direction for something nobody asked
// for.
func (s *Service) spendInitiative(guildID string, t mind.Trigger, now time.Time) {
	err := s.store.UpdateMindGuild(guildID, func(g *storage.MindGuild) {
		f := mind.Fatigue{Level: g.Initiative, At: g.InitiativeAt}.Spend(t, now)
		g.Initiative, g.InitiativeAt = f.Level, f.At
	})
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_initiative_record_failed")
	}
}

// initiativeNote is the fatigue she decided under, for the journal's rule,
// or nothing when she was fresh.
func initiativeNote(fatigue float64) string {
	if fatigue < noticeableFatigue {
		return ""
	}
	return fmt.Sprintf(" (already put herself forward lately: fatigue %.2f)", fatigue)
}

// noticeableFatigue is where fatigue is worth mentioning in the journal.
const noticeableFatigue = 0.05
