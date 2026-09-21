package memory

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Person is her dossier on someone: who they are to her, where things stand
// between them, and dated notes she has taken.
//
// Who and Between are paragraphs she rewrites when she reflects; Notes are
// added as she learns things and folded into those paragraphs later. That is
// the whole of how she feels about someone — there is no score. See
// docs/persona.md.
type Person struct {
	ID   string
	Name string
	// FirstMet and LastTalked are when she first and last spoke with them.
	FirstMet   time.Time
	LastTalked time.Time
	// Feeling is how she feels about them right now, in a few words.
	Feeling string
	// Who is what she knows about them; Between is how things stand
	// between the two of them.
	Who     string
	Between string
	// Kept is what stays with her about them: the few moments that define
	// them for her, chosen and replaced when she reflects. See MaxKept.
	Kept  []Note
	Notes []Note
}

// Note is one dated thing she noted about someone.
type Note struct {
	Day  time.Time
	Text string
}

// Person sections and front matter keys. Headings are what a person reading
// the file sees, so they are words rather than identifiers.
const (
	keyName       = "name"
	keyID         = "id"
	keyFirstMet   = "first_met"
	keyLastTalked = "last_talked"
	keyFeeling    = "feeling"

	headWho     = "who they are"
	headBetween = "between us"
	headKept    = "what stays with me"
	headNotes   = "notes"
)

// MaxKept is how many moments stay with her about one person. A handful,
// the way people remember each other: a few defining episodes and a general
// sense, rather than a log.
const MaxKept = 5

// MaxNotes is how many dated notes a dossier keeps. Reflection folds what
// matters into the paragraphs above them, so the oldest are dropped rather
// than kept forever; a dossier that grows without bound ends up in front of
// the model as a wall.
const MaxNotes = 20

// Person reads her dossier on someone. The bool is false when she has none.
func (s *Store) Person(guildID, userID string) (Person, bool, error) {
	path, err := s.personPath(guildID, userID)
	if err != nil {
		return Person{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := readPerson(path, s.loc)
	return p, p.ID != "", err
}

// UpdatePerson reads, changes and writes back a dossier as one step, starting
// one if there is none.
func (s *Store) UpdatePerson(guildID, userID string, change func(*Person)) error {
	path, err := s.personPath(guildID, userID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := readPerson(path, s.loc)
	if err != nil {
		return err
	}
	p.ID = userID
	change(&p)
	if len(p.Kept) > MaxKept {
		p.Kept = p.Kept[len(p.Kept)-MaxKept:]
	}
	if len(p.Notes) > MaxNotes {
		p.Notes = p.Notes[len(p.Notes)-MaxNotes:]
	}
	return writeFile(path, renderPerson(p, s.loc))
}

// People lists every dossier in a guild.
func (s *Store) People(guildID string) ([]Person, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(filepath.Join(dir, peopleDir))
	if err != nil {
		return nil, nil
	}
	var out []Person
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p, err := readPerson(filepath.Join(dir, peopleDir, e.Name()), s.loc)
		if err == nil && p.ID != "" {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastTalked.After(out[j].LastTalked) })
	return out, nil
}

func (s *Store) personPath(guildID, userID string) (string, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return "", err
	}
	if !safeName(userID) {
		return "", errBadID(userID)
	}
	return filepath.Join(dir, peopleDir, userID+".md"), nil
}

func readPerson(path string, loc *time.Location) (Person, error) {
	text, err := readFile(path)
	if err != nil || text == "" {
		return Person{}, err
	}
	fields, body := parseDoc(text)
	parts := sections(body)
	p := Person{
		ID:         fields[keyID],
		Name:       fields[keyName],
		FirstMet:   parseTime(fields[keyFirstMet]),
		LastTalked: parseTime(fields[keyLastTalked]),
		Feeling:    fields[keyFeeling],
		Who:        parts[headWho],
		Between:    parts[headBetween],
	}
	if p.ID == "" {
		p.ID = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	for _, item := range bullets(parts[headKept]) {
		p.Kept = append(p.Kept, parseNote(item, loc))
	}
	for _, item := range bullets(parts[headNotes]) {
		p.Notes = append(p.Notes, parseNote(item, loc))
	}
	return p, nil
}

// parseNote reads "2026-09-19 text". A note without a date — one a person
// added by hand — keeps its text and simply has no day.
func parseNote(item string, loc *time.Location) Note {
	if len(item) > len(dayLayout) && item[len(dayLayout)] == ' ' {
		if day, err := time.ParseInLocation(dayLayout, item[:len(dayLayout)], loc); err == nil {
			return Note{Day: day, Text: strings.TrimSpace(item[len(dayLayout)+1:])}
		}
	}
	return Note{Text: item}
}

// writeNotes writes a section of dated bullets, or nothing when empty.
func writeNotes(b *strings.Builder, heading string, notes []Note, loc *time.Location) {
	if len(notes) == 0 {
		return
	}
	b.WriteString("## " + heading + "\n\n")
	for _, n := range notes {
		b.WriteString("- ")
		if !n.Day.IsZero() {
			b.WriteString(n.Day.In(loc).Format(dayLayout) + " ")
		}
		b.WriteString(oneLine(n.Text) + "\n")
	}
	b.WriteString("\n")
}

func renderPerson(p Person, loc *time.Location) string {
	var body strings.Builder
	if w := strings.TrimSpace(p.Who); w != "" {
		body.WriteString("## Who they are\n\n" + w + "\n\n")
	}
	if b := strings.TrimSpace(p.Between); b != "" {
		body.WriteString("## Between us\n\n" + b + "\n\n")
	}
	writeNotes(&body, "What stays with me", p.Kept, loc)
	writeNotes(&body, "Notes", p.Notes, loc)
	return renderDoc([]field{
		{keyName, p.Name},
		{keyID, p.ID},
		{keyFirstMet, formatTime(p.FirstMet)},
		{keyLastTalked, formatTime(p.LastTalked)},
		{keyFeeling, p.Feeling},
	}, body.String())
}
