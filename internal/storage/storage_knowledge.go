package storage

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// UpsertKnowledgeDoc adds or replaces a doc by slug. Body is capped so one
// paste cannot blow up the transcript budget downstream.
func (s *Storage) UpsertKnowledgeDoc(guildID, title, body, source string) (*KnowledgeDoc, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" || body == "" {
		return nil, fmt.Errorf("storage: title and body are required")
	}
	if len(body) > 4000 {
		body = body[:4000]
	}
	doc := &KnowledgeDoc{
		GuildID:   guildID,
		Slug:      Slugify(title),
		Title:     title,
		Body:      body,
		Source:    strings.TrimSpace(source),
		UpdatedAt: time.Now(),
	}
	if doc.Slug == "" {
		return nil, fmt.Errorf("storage: title has no usable characters")
	}
	if err := s.knowledge.Put(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// DeleteKnowledgeDoc removes one doc by title slug. Missing is not an error.
func (s *Storage) DeleteKnowledgeDoc(guildID, title string) error {
	slug := Slugify(title)
	if slug == "" {
		return fmt.Errorf("storage: title has no usable characters")
	}
	return s.knowledge.Delete(guildScopedKey(guildID, slug))
}

// ListKnowledgeDocs returns the guild's docs oldest-update first.
func (s *Storage) ListKnowledgeDocs(guildID string) []KnowledgeDoc {
	rows := s.knowledgeByGuild.Find(guildID)
	out := make([]KnowledgeDoc, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Slug < out[j].Slug
		}
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	return out
}

// Slugify turns a title into a key-safe slug.
func Slugify(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	prevDash := true
	for _, r := range t {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
		if b.Len() >= 60 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
