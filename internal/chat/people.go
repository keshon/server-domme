package chat

import (
	"context"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// warmTo carries a conversation's tone into how much she likes the people in
// it. No backend call: the tone was already read by the summary.
func (s *Service) warmTo(guildID string, people []string, tone mind.Tone, at time.Time) {
	step := mind.WarmthStep(tone, len(people) == 1)
	if step == 0 {
		return
	}
	for _, userID := range people {
		var current float64
		if p := s.store.GetMindPerson(guildID, userID); p != nil {
			current = mind.WarmthNow(p.Warmth, p.WarmAt, at)
		}
		level := current + step
		if level < 0 {
			level = 0
		}
		if level > 1 {
			level = 1
		}
		if err := s.store.WarmMindPerson(guildID, userID, level, at); err != nil {
			s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_warmth_record_failed")
		}
	}
}

// notePeople updates the person file for everyone in a remembered
// conversation: facts they stated about themselves, and her impression.
//
// One backend call per remembered conversation, separate from the summary so
// a relay that mangles one format does not cost the other. Best-effort like
// the summary: a failure means nothing new was learned this time, which
// nobody can see.
func (s *Service) notePeople(ctx context.Context, guildID string, turns []mind.Turn, at time.Time) {
	ids := byName(turns)
	if len(ids) == 0 {
		return
	}

	var known []mind.PersonNote
	for name, userID := range ids {
		p := s.store.GetMindPerson(guildID, userID)
		if p == nil {
			continue
		}
		known = append(known, mind.PersonNote{
			Name:       name,
			Impression: p.Impression,
			Facts:      factsOf(p),
		})
	}

	callCtx, cancel := context.WithTimeout(ctx, rememberTimeout)
	reply, err := s.provider.Generate(callCtx, mind.NotesPrompt(s.persona(), turns, known))
	cancel()
	if err != nil {
		s.log.Debug().Err(err).Str("guild_id", guildID).Msg("chat_notes_generate_failed")
		return
	}

	for _, update := range mind.ParseNotes(reply, at) {
		// Only people who were actually in the conversation. A name the model
		// made up, or someone who was merely mentioned, is not someone to
		// keep a file on.
		userID, ok := ids[strings.ToLower(update.Name)]
		if !ok {
			continue
		}

		var previous []mind.Fact
		if p := s.store.GetMindPerson(guildID, userID); p != nil {
			previous = factsOf(p)
		}
		facts := mind.MergeFacts(previous, update.Facts)

		if err := s.store.NoteMindPerson(guildID, userID, toStored(facts), update.Impression, at); err != nil {
			s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_notes_store_failed")
			continue
		}
		s.log.Info().
			Str("guild_id", guildID).
			Str("user_id", userID).
			Int("facts_learned", len(update.Facts)).
			Bool("impression", update.Impression != "").
			Msg("chat_noted_person")
	}
}

// persona is the character's own description, or "" without a character.
func (s *Service) persona() string {
	if s.character == nil {
		return ""
	}
	return s.character.Persona
}

// byName maps the lowercased name of everyone who spoke to their id, which is
// how a line in the notes reply is tied back to a person.
func byName(turns []mind.Turn) map[string]string {
	out := make(map[string]string)
	for _, t := range turns {
		if t.FromBot || t.UserID == "" || strings.TrimSpace(t.Username) == "" {
			continue
		}
		out[strings.ToLower(t.Username)] = t.UserID
	}
	return out
}

func factsOf(p *storage.MindPerson) []mind.Fact {
	out := make([]mind.Fact, 0, len(p.Facts))
	for _, f := range p.Facts {
		out = append(out, mind.Fact{Key: f.Key, Value: f.Value, At: f.At})
	}
	return out
}

func toStored(facts []mind.Fact) []storage.MindFact {
	out := make([]storage.MindFact, 0, len(facts))
	for _, f := range facts {
		out = append(out, storage.MindFact{Key: f.Key, Value: f.Value, At: f.At})
	}
	return out
}
