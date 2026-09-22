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
			if err != nil || len(day.Moments) == 0 {
				continue
			}
			key := guildID + ":" + date.Format("2006-01-02")
			// Two passes, each retried and failing on its own, so a bad
			// answer costs one part of a day rather than all of it. See
			// docs/persona-v3.md, Known failure points.
			if day.Summary == "" && s.tryReflect(key) {
				s.reflectPass(ctx, guildID, key, "day", func(ctx context.Context) (bool, error) {
					return s.mind.Reflect(ctx, guildID, name, date, now, s.roomRates(guildID, date))
				})
			}
			if s.mind.SelfFacts && s.selfFactsDue(guildID, date) && s.tryReflect(key+":self") {
				s.reflectPass(ctx, guildID, key, "self", func(ctx context.Context) (bool, error) {
					return s.mind.ReflectSelf(ctx, guildID, date)
				})
			}
		}
	}
}

// reflectPass runs one pass of reflection on a day, and says how it went.
func (s *Service) reflectPass(ctx context.Context, guildID, key, pass string, run func(context.Context) (bool, error)) {
	genCtx, cancel := context.WithTimeout(ctx, s.generateTimeout)
	defer cancel()
	did, err := run(genCtx)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Str("day", key).Str("pass", pass).Msg("chat_reflect_failed")
		return
	}
	if did {
		s.log.Info().Str("guild_id", guildID).Str("day", key).Str("pass", pass).Msg("chat_reflected")
	}
}

// selfFactsDue reports whether her words on a day have yet to be read for
// facts about herself.
func (s *Service) selfFactsDue(guildID string, date time.Time) bool {
	me, err := s.memory.Me(guildID)
	if err != nil {
		return false
	}
	y, m, d := date.Date()
	return me.Through.Before(time.Date(y, m, d, 0, 0, 0, 0, date.Location()))
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
