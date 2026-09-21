package chat

import (
	"context"
	"time"
)

// Reflection. Once the day has turned — past ReflectHour in the community's
// timezone — she looks back on each of the last few days she has not yet
// made sense of. Catching up on several is what makes a bot that was down
// overnight still reflect; stopping at reflectBack is what keeps a first
// start on an old memory directory from spending a call per day of it.
const (
	reflectEvery    = 15 * time.Minute
	reflectBack     = 3
	reflectAttempts = 3
)

func (s *Service) reflectLoop(ctx context.Context) {
	ticker := time.NewTicker(reflectEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reflectDue(ctx)
		}
	}
}

// reflectDue reflects on every day that is due, in every guild she
// remembers.
func (s *Service) reflectDue(ctx context.Context) {
	now := s.now().In(s.location)
	if now.Hour() < s.reflectHour {
		return
	}
	sess := s.session()
	for _, guildID := range s.memory.Guilds() {
		name := guildID
		if sess != nil && sess.State != nil {
			if g, err := sess.State.Guild(guildID); err == nil && g != nil {
				name = g.Name
			}
		}
		for back := reflectBack; back >= 1; back-- {
			if ctx.Err() != nil {
				return
			}
			date := now.AddDate(0, 0, -back)
			day, err := s.memory.Day(guildID, date)
			if err != nil || day.Summary != "" || len(day.Moments) == 0 {
				continue
			}
			key := guildID + ":" + date.Format("2006-01-02")
			if !s.tryReflect(key) {
				continue
			}
			genCtx, cancel := context.WithTimeout(ctx, s.generateTimeout)
			did, err := s.mind.Reflect(genCtx, guildID, name, date, now)
			cancel()
			if err != nil {
				s.log.Warn().Err(err).Str("guild_id", guildID).Str("day", key).Msg("chat_reflect_failed")
				continue
			}
			if did {
				s.log.Info().Str("guild_id", guildID).Str("day", key).Msg("chat_reflected")
			}
		}
	}
}

// tryReflect counts an attempt at a day and reports whether it may be made.
// A day the model cannot make sense of three times running is left
// unreflected rather than retried every quarter of an hour forever.
func (s *Service) tryReflect(key string) bool {
	s.reflectMu.Lock()
	defer s.reflectMu.Unlock()
	if s.reflected[key] >= reflectAttempts {
		return false
	}
	s.reflected[key]++
	return true
}
