package memory

import (
	"path/filepath"
	"strings"
	"time"
)

// SelfFact is something she said about herself, which has become true of
// her: "hates Rust — told Big M". It comes only from a message she actually
// sent; see docs/persona-v3.md, F2 and I.
type SelfFact struct {
	Day    time.Time
	Text   string
	Source Source
	// Conflict is the line of her character card this contradicts, when it
	// does. The card stays canon and the fact stays as history; both are
	// shown to her, and the conflict waits for the author to resolve.
	Conflict string
	// Superseded is when she said otherwise and the fact stopped being
	// current. A superseded fact is kept, never shown to her again.
	Superseded time.Time
}

// Me is what she has said about herself in one guild.
type Me struct {
	// Facts are current: shown to her when they bear on the conversation.
	Facts []SelfFact
	// Superseded are facts she has changed her mind about.
	Superseded []SelfFact
	// Through is the last day her own words were read for facts.
	Through time.Time
}

// Conflicts are the current facts that contradict the card.
func (m Me) Conflicts() []SelfFact {
	var out []SelfFact
	for _, f := range m.Facts {
		if f.Conflict != "" {
			out = append(out, f)
		}
	}
	return out
}

// MaxSelfFacts is how many current facts she keeps. Past it the oldest are
// let go: a person does not carry every opinion they ever voiced.
const MaxSelfFacts = 40

// maxSuperseded is how many superseded facts stay in the file, for a person
// reading it to see where she changed her mind.
const maxSuperseded = 40

// me.md layout.
const (
	meFile         = "me.md"
	keyThrough     = "through"
	headFacts      = "facts"
	headConflicts  = "in conflict with the card"
	headSuperseded = "superseded"
	conflictTag    = "card: "
	supersededTag  = "superseded "
)

// Me reads what she has said about herself in a guild.
func (s *Store) Me(guildID string) (Me, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return Me{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return readMe(filepath.Join(dir, meFile), s.loc)
}

// UpdateMe reads, changes and writes back what she has said about herself,
// as one step.
func (s *Store) UpdateMe(guildID string, change func(*Me)) error {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(dir, meFile)
	me, err := readMe(path, s.loc)
	if err != nil {
		return err
	}
	change(&me)
	if len(me.Facts) > MaxSelfFacts {
		me.Facts = me.Facts[len(me.Facts)-MaxSelfFacts:]
	}
	if len(me.Superseded) > maxSuperseded {
		me.Superseded = me.Superseded[len(me.Superseded)-maxSuperseded:]
	}
	return writeFile(path, renderMe(me, s.loc))
}

func readMe(path string, loc *time.Location) (Me, error) {
	text, err := readFile(path)
	if err != nil || text == "" {
		return Me{}, err
	}
	fields, body := parseDoc(text)
	parts := sections(body)
	me := Me{}
	if t, err := time.ParseInLocation(dayLayout, fields[keyThrough], loc); err == nil {
		me.Through = t
	}
	for _, head := range []string{headFacts, headConflicts} {
		for _, item := range bullets(parts[head]) {
			if f, ok := parseSelfFact(item, loc); ok {
				me.Facts = append(me.Facts, f)
			}
		}
	}
	for _, item := range bullets(parts[headSuperseded]) {
		if f, ok := parseSelfFact(item, loc); ok {
			me.Superseded = append(me.Superseded, f)
		}
	}
	return me, nil
}

// parseSelfFact reads "2026-09-22 hates Rust · stated msg 1234", with an
// optional "· card: …" or "· superseded 2026-10-01" after the source.
func parseSelfFact(item string, loc *time.Location) (SelfFact, bool) {
	parts := strings.Split(item, sourceSep)
	at := -1
	var f SelfFact
	for i := len(parts) - 1; i >= 1; i-- {
		if src, ok := ParseSource(parts[i]); ok {
			f.Source, at = src, i
			break
		}
	}
	head := item
	if at > 0 {
		head = strings.Join(parts[:at], sourceSep)
		for _, extra := range parts[at+1:] {
			extra = strings.TrimSpace(extra)
			switch {
			case strings.HasPrefix(extra, conflictTag):
				f.Conflict = strings.TrimSpace(strings.TrimPrefix(extra, conflictTag))
			case strings.HasPrefix(extra, supersededTag):
				f.Superseded, _ = time.ParseInLocation(dayLayout, strings.TrimPrefix(extra, supersededTag), loc)
			}
		}
	}
	n := parseNote(head, loc)
	f.Day, f.Text = n.Day, n.Text
	return f, f.Text != ""
}

func renderSelfFact(f SelfFact, loc *time.Location) string {
	var b strings.Builder
	if !f.Day.IsZero() {
		b.WriteString(f.Day.In(loc).Format(dayLayout) + " ")
	}
	b.WriteString(withSource(f.Text, f.Source))
	if f.Source.IsZero() && (f.Conflict != "" || !f.Superseded.IsZero()) {
		// Without a source the extras would be read back as part of the
		// text; a fact with no source is one a person wrote by hand.
		return b.String()
	}
	if f.Conflict != "" {
		b.WriteString(sourceSep + conflictTag + oneLine(f.Conflict))
	}
	if !f.Superseded.IsZero() {
		b.WriteString(sourceSep + supersededTag + f.Superseded.In(loc).Format(dayLayout))
	}
	return b.String()
}

func renderMe(me Me, loc *time.Location) string {
	var body strings.Builder
	list := func(heading string, facts []SelfFact) {
		if len(facts) == 0 {
			return
		}
		body.WriteString("## " + heading + "\n\n")
		for _, f := range facts {
			body.WriteString("- " + renderSelfFact(f, loc) + "\n")
		}
		body.WriteString("\n")
	}
	var plain, conflicts []SelfFact
	for _, f := range me.Facts {
		if f.Conflict != "" {
			conflicts = append(conflicts, f)
		} else {
			plain = append(plain, f)
		}
	}
	list("Facts", plain)
	list("In conflict with the card", conflicts)
	list("Superseded", me.Superseded)

	through := ""
	if !me.Through.IsZero() {
		through = me.Through.In(loc).Format(dayLayout)
	}
	return renderDoc([]field{{keyThrough, through}}, body.String())
}
