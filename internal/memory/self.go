package memory

import (
	"path/filepath"
	"time"
)

// Self is how she sees herself in one guild lately, and how she is feeling.
//
// Lately is rewritten by reflection, once a day; Mood is replaced by every
// appraisal. They are kept apart because they move at different speeds: a
// mood is gone by the evening, what has been going on in her life is not.
type Self struct {
	// Lately is her own account of what has been going on with her, in the
	// first person. Empty until the first reflection; see Seed.
	Lately string
	// Mood is a few words, and MoodAt when they were true. Read with its
	// age: a mood from this morning is not a mood now.
	Mood   string
	MoodAt time.Time
	// Reflected is when Lately was last rewritten.
	Reflected time.Time
}

// Front matter keys for self.md.
const (
	keyMood      = "mood"
	keyMoodAt    = "mood_at"
	keyReflected = "reflected"
)

// Self reads how she sees herself in a guild. A guild she has never been in
// yields the zero Self, not an error.
func (s *Store) Self(guildID string) (Self, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return Self{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return readSelf(filepath.Join(dir, selfFile))
}

func readSelf(path string) (Self, error) {
	text, err := readFile(path)
	if err != nil || text == "" {
		return Self{}, err
	}
	fields, body := parseDoc(text)
	return Self{
		Lately:    body,
		Mood:      fields[keyMood],
		MoodAt:    parseTime(fields[keyMoodAt]),
		Reflected: parseTime(fields[keyReflected]),
	}, nil
}

// UpdateSelf reads, changes and writes back how she sees herself, as one
// step.
func (s *Store) UpdateSelf(guildID string, change func(*Self)) error {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(dir, selfFile)
	self, err := readSelf(path)
	if err != nil {
		return err
	}
	change(&self)
	return writeFile(path, renderDoc([]field{
		{keyMood, self.Mood},
		{keyMoodAt, formatTime(self.MoodAt)},
		{keyReflected, formatTime(self.Reflected)},
	}, self.Lately))
}
