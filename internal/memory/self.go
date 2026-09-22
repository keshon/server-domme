package memory

import (
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Self is how she sees herself in one guild lately, and how she is feeling.
//
// Lately is rewritten by reflection, once a day; Mood is replaced by every
// appraisal. They are kept apart because they move at different speeds: a
// mood is gone by the evening, what has been going on in her life is not.
//
// v3 adds Feelings, which replace Mood when they are on: several at once,
// each with what it is about, fading on its own clock. See
// docs/persona-v3.md, C.
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
	// Feelings are what is still with her, each fading on its own.
	Feelings []Feeling
}

// Feeling is one thing she feels, and what about: "stung", about "Big M's
// jab about my taste". The model names both; the person, the time and the
// source are anchored by the code from the moment it came from.
type Feeling struct {
	At     time.Time
	Person Ref
	What   string
	About  string
	// Weight is how much it got to her when it happened, 0 to 1. It fades
	// from there; see Strength.
	Weight float64
	Source Source
}

// Fading. A feeling's strength falls from its weight exponentially, and
// heavier feelings fade slower: about two hours at 0.1, a day at 0.9. Below
// FeelingGone it is gone.
const (
	fadeLight   = 2 * time.Hour
	fadeHeavy   = 24 * time.Hour
	FeelingGone = 0.1
	// MaxFeelings is how many are held at once.
	MaxFeelings = 5
)

// Strength is how strong a feeling still is at now.
func (f Feeling) Strength(now time.Time) float64 {
	age := now.Sub(f.At)
	if age < 0 {
		age = 0
	}
	w := math.Max(0, math.Min(1, f.Weight))
	tau := fadeLight + time.Duration((w-0.1)/0.8*float64(fadeHeavy-fadeLight))
	tau = max(fadeLight, min(fadeHeavy, tau))
	return w * math.Exp(-float64(age)/float64(tau))
}

// Live are the feelings still with her at now, most recent first.
func Live(feelings []Feeling, now time.Time) []Feeling {
	var out []Feeling
	for _, f := range feelings {
		if f.Strength(now) >= FeelingGone {
			out = append(out, f)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Front matter keys and sections for self.md.
const (
	keyMood      = "mood"
	keyMoodAt    = "mood_at"
	keyReflected = "reflected"
	headFeelings = "feelings"
)

// feelingLine reads "2026-09-22 14:05 [Big M:123] stung · about his jab ·
// weight 0.6 · interpreted msg 555", the person optional.
var feelingLine = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{1,2}:\d{2})\s+(?:\[([^\]]*)\]\s+)?(.*)$`)

// Self reads how she sees herself in a guild. A guild she has never been in
// yields the zero Self, not an error.
func (s *Store) Self(guildID string) (Self, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return Self{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return readSelf(filepath.Join(dir, selfFile), s.loc)
}

func readSelf(path string, loc *time.Location) (Self, error) {
	text, err := readFile(path)
	if err != nil || text == "" {
		return Self{}, err
	}
	fields, body := parseDoc(text)
	parts := sections(body)
	self := Self{
		Lately:    parts[""],
		Mood:      fields[keyMood],
		MoodAt:    parseTime(fields[keyMoodAt]),
		Reflected: parseTime(fields[keyReflected]),
	}
	for _, item := range bullets(parts[headFeelings]) {
		if f, ok := parseFeeling(item, loc); ok {
			self.Feelings = append(self.Feelings, f)
		}
	}
	return self, nil
}

func parseFeeling(item string, loc *time.Location) (Feeling, bool) {
	m := feelingLine.FindStringSubmatch(item)
	if m == nil {
		return Feeling{}, false
	}
	at, err := time.ParseInLocation(minuteLayout, m[1], loc)
	if err != nil {
		return Feeling{}, false
	}
	f := Feeling{At: at}
	if ref := strings.TrimSpace(m[2]); ref != "" {
		if i := strings.LastIndex(ref, ":"); i > 0 {
			f.Person = Ref{Name: strings.TrimSpace(ref[:i]), ID: strings.TrimSpace(ref[i+1:])}
		} else {
			f.Person = Ref{Name: ref}
		}
	}
	for i, part := range strings.Split(m[3], sourceSep) {
		part = strings.TrimSpace(part)
		switch {
		case i == 0:
			f.What = part
		case strings.HasPrefix(part, "about "):
			f.About = strings.TrimPrefix(part, "about ")
		case strings.HasPrefix(part, weightTag):
			f.Weight, _ = strconv.ParseFloat(strings.TrimPrefix(part, weightTag), 64)
		default:
			if src, ok := ParseSource(part); ok {
				f.Source = src
			}
		}
	}
	return f, f.What != ""
}

func renderFeeling(f Feeling, loc *time.Location) string {
	var b strings.Builder
	b.WriteString(f.At.In(loc).Format(minuteLayout) + " ")
	if f.Person.Name != "" || f.Person.ID != "" {
		name := strings.NewReplacer("[", "(", "]", ")").Replace(oneLine(f.Person.Name))
		if f.Person.ID != "" {
			name += ":" + f.Person.ID
		}
		b.WriteString("[" + name + "] ")
	}
	b.WriteString(oneLine(f.What))
	if f.About != "" {
		b.WriteString(sourceSep + "about " + oneLine(f.About))
	}
	b.WriteString(sourceSep + weightTag + strconv.FormatFloat(f.Weight, 'f', 2, 64))
	if src := f.Source.String(); src != "" {
		b.WriteString(sourceSep + src)
	}
	return b.String()
}

// UpdateSelf reads, changes and writes back how she sees herself, as one
// step. At most MaxFeelings are kept, the newest; letting faded ones go is
// the caller's, since only the caller knows what time it is. See Live.
func (s *Store) UpdateSelf(guildID string, change func(*Self)) error {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(dir, selfFile)
	self, err := readSelf(path, s.loc)
	if err != nil {
		return err
	}
	change(&self)

	if len(self.Feelings) > MaxFeelings {
		self.Feelings = self.Feelings[len(self.Feelings)-MaxFeelings:]
	}

	var body strings.Builder
	body.WriteString(strings.TrimSpace(self.Lately))
	if len(self.Feelings) > 0 {
		body.WriteString("\n\n## Feelings\n\n")
		for _, f := range self.Feelings {
			body.WriteString("- " + renderFeeling(f, s.loc) + "\n")
		}
	}
	return writeFile(path, renderDoc([]field{
		{keyMood, self.Mood},
		{keyMoodAt, formatTime(self.MoodAt)},
		{keyReflected, formatTime(self.Reflected)},
	}, body.String()))
}
