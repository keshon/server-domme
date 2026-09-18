package chat

import (
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// concernsOf reads a person's concerns out of their record.
func concernsOf(p *storage.MindPerson) []mind.Concern {
	if p == nil {
		return nil
	}
	out := make([]mind.Concern, 0, len(p.Concerns))
	for _, c := range p.Concerns {
		out = append(out, mind.Concern{What: c.What, Due: c.Due, Expires: c.Expires, Noted: c.Noted, Passed: c.Passed})
	}
	return out
}

func toStoredConcerns(cs []mind.Concern) []storage.MindConcern {
	out := make([]storage.MindConcern, 0, len(cs))
	for _, c := range cs {
		out = append(out, storage.MindConcern{What: c.What, Due: c.Due, Expires: c.Expires, Noted: c.Noted, Passed: c.Passed})
	}
	return out
}

// noteConcerns dates and keeps the plans the notes call found for someone,
// if they are in that person's own words.
func (s *Service) noteConcerns(guildID, userID string, plans []mind.Plan, turns []mind.Turn, at time.Time) {
	if len(plans) == 0 {
		return
	}
	var theirs []string
	for _, t := range turns {
		if t.UserID == userID {
			theirs = append(theirs, t.Content)
		}
	}
	var noted []mind.Concern
	for _, p := range plans {
		c, ok := mind.NewConcern(p, at, s.loc(), theirs)
		if !ok {
			s.log.Info().Str("guild_id", guildID).Str("user_id", userID).Str("plan", p.What).Msg("chat_plan_rejected")
			continue
		}
		noted = append(noted, c)
	}
	if len(noted) == 0 {
		return
	}
	err := s.store.UpdateMindPerson(guildID, userID, at, func(p *storage.MindPerson) {
		p.Concerns = toStoredConcerns(mind.MergeConcerns(concernsOf(p), noted, at))
	})
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_concern_store_failed")
		return
	}
	s.log.Info().Str("guild_id", guildID).Str("user_id", userID).Int("plans", len(noted)).Msg("chat_concern_noted")
}

// concernOnMind picks, when she is answering someone, whether something they
// told her is on her mind now: the most pressing concern, rolled against its
// salience. Returns what it is and the phrase for the prompt, or empty.
func (s *Service) concernOnMind(guildID, userID, name string, mood float64, now time.Time) (string, string) {
	p := s.store.GetMindPerson(guildID, userID)
	cs := concernsOf(p)
	if len(cs) == 0 {
		return "", ""
	}
	closeness, _, _ := bondOf(p).Now(now)
	i, salience := mind.MostPressing(cs, now, closeness, mood)
	if i < 0 || s.roll() >= salience {
		return "", ""
	}
	return cs[i].What, cs[i].Phrase(name, now, s.loc())
}

// afterConcern learns from her reply whether she brought up what was on her
// mind. If she did, it is done with; if she let it pass, it weakens.
func (s *Service) afterConcern(guildID, userID, what, reply string, now time.Time) {
	if what == "" {
		return
	}
	raised := (mind.Concern{What: what}).Mentions(reply)
	err := s.store.UpdateMindPerson(guildID, userID, now, func(p *storage.MindPerson) {
		kept := p.Concerns[:0]
		for _, c := range p.Concerns {
			if c.What == what {
				if raised {
					continue
				}
				c.Passed++
			}
			kept = append(kept, c)
		}
		p.Concerns = kept
	})
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_concern_store_failed")
		return
	}
	s.log.Info().
		Str("guild_id", guildID).
		Str("user_id", userID).
		Str("concern", what).
		Bool("raised", raised).
		Msg("chat_concern_on_her_mind")
}

// concernRaisedAhead is how long before it happens a plan counts as being
// talked about as having happened. Before that, their mentioning it again is
// just more of the plan.
const concernRaisedAhead = 6 * time.Hour

// resolveConcerns closes whatever someone's own message says they have got
// round to: the world answering the question before she asks it.
func (s *Service) resolveConcerns(guildID, userID, content string, person *storage.MindPerson, now time.Time) {
	if person == nil || len(person.Concerns) == 0 {
		return
	}
	closing := false
	for _, c := range concernsOf(person) {
		if now.After(c.Due.Add(-concernRaisedAhead)) && c.Mentions(content) {
			closing = true
		}
	}
	if !closing {
		return
	}
	err := s.store.UpdateMindPerson(guildID, userID, now, func(p *storage.MindPerson) {
		kept := p.Concerns[:0]
		for i, c := range concernsOf(p) {
			if now.After(c.Due.Add(-concernRaisedAhead)) && c.Mentions(content) {
				continue
			}
			kept = append(kept, p.Concerns[i])
		}
		p.Concerns = kept
	})
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_concern_store_failed")
		return
	}
	s.log.Info().Str("guild_id", guildID).Str("user_id", userID).Msg("chat_concern_resolved")
}
