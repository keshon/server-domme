package memory

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Moment is one thing that happened, as she would put it: what was said or
// done, where, and who was there.
//
// Her own messages are moments too, with what she meant by them. That is
// what lets her own a thing she said yesterday instead of reading it back as
// somebody else's words — the failure that retired v1. See docs/persona.md.
type Moment struct {
	At      time.Time
	Channel string
	People  []Ref
	Text    string
}

// Ref is a person as a moment names them: the id that finds their dossier and
// the name she knew them by then.
type Ref struct {
	ID   string
	Name string
}

// Day is one day file: a summary written when she reflected on it, and the
// moments, in order.
type Day struct {
	Date    time.Time
	Summary string
	Moments []Moment
}

// Day file headings.
const (
	headSummary = "summary"
	headMoments = "moments"
)

// momentLine reads "14:05 [#chat; Big M:123] text". The bracket is optional
// so a line a person added by hand still reads.
var momentLine = regexp.MustCompile(`^(\d{1,2}:\d{2})\s+(?:\[([^\]]*)\]\s+)?(.*)$`)

// AddMoment appends a moment to the day it happened on.
func (s *Store) AddMoment(guildID string, m Moment) error {
	if strings.TrimSpace(m.Text) == "" {
		return nil
	}
	dir, err := s.guildDir(guildID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	at := m.At.In(s.loc)
	path := dayPath(dir, at)
	day, err := readDay(path, s.loc)
	if err != nil {
		return err
	}
	day.Date = startOfDay(at)
	day.Moments = append(day.Moments, m)
	sort.SliceStable(day.Moments, func(i, j int) bool { return day.Moments[i].At.Before(day.Moments[j].At) })
	return writeFile(path, renderDay(day, s.loc))
}

// SetSummary writes what she made of a day when she reflected on it.
func (s *Store) SetSummary(guildID string, date time.Time, summary string) error {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	date = date.In(s.loc)
	path := dayPath(dir, date)
	day, err := readDay(path, s.loc)
	if err != nil {
		return err
	}
	day.Date = startOfDay(date)
	day.Summary = strings.TrimSpace(summary)
	return writeFile(path, renderDay(day, s.loc))
}

// Day reads one day. A day with nothing in it yields an empty Day.
func (s *Store) Day(guildID string, date time.Time) (Day, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return Day{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	date = date.In(s.loc)
	day, err := readDay(dayPath(dir, date), s.loc)
	day.Date = startOfDay(date)
	return day, err
}

// Days reads the days with anything in them from the last n days up to and
// including now's, oldest first.
func (s *Store) Days(guildID string, now time.Time, n int) ([]Day, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Day
	today := startOfDay(now.In(s.loc))
	for i := n - 1; i >= 0; i-- {
		date := today.AddDate(0, 0, -i)
		day, err := readDay(dayPath(dir, date), s.loc)
		if err != nil {
			return nil, err
		}
		if day.Summary == "" && len(day.Moments) == 0 {
			continue
		}
		day.Date = date
		out = append(out, day)
	}
	return out, nil
}

// DayFiles lists the dates that have a day file, oldest first.
func (s *Store) DayFiles(guildID string) []time.Time {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(dir, daysDir))
	if err != nil {
		return nil
	}
	var out []time.Time
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if d, err := time.ParseInLocation(dayLayout, name, s.loc); err == nil {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

func dayPath(dir string, date time.Time) string {
	return filepath.Join(dir, daysDir, date.Format(dayLayout)+".md")
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func readDay(path string, loc *time.Location) (Day, error) {
	text, err := readFile(path)
	if err != nil || text == "" {
		return Day{}, err
	}
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	date, _ := time.ParseInLocation(dayLayout, name, loc)

	parts := sections(text)
	day := Day{Date: date, Summary: parts[headSummary]}
	for _, item := range bullets(parts[headMoments]) {
		if m, ok := parseMoment(item, date, loc); ok {
			day.Moments = append(day.Moments, m)
		}
	}
	return day, nil
}

func parseMoment(item string, date time.Time, loc *time.Location) (Moment, bool) {
	match := momentLine.FindStringSubmatch(item)
	if match == nil {
		return Moment{}, false
	}
	clock, err := time.ParseInLocation(clockLayout, match[1], loc)
	if err != nil {
		return Moment{}, false
	}
	m := Moment{
		At:   time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, loc),
		Text: strings.TrimSpace(match[3]),
	}
	for _, part := range strings.Split(match[2], ";") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case strings.HasPrefix(part, "#"):
			m.Channel = strings.TrimPrefix(part, "#")
		default:
			// The id is after the last colon: a name may contain one, an id
			// never does.
			if i := strings.LastIndex(part, ":"); i > 0 {
				m.People = append(m.People, Ref{Name: strings.TrimSpace(part[:i]), ID: strings.TrimSpace(part[i+1:])})
			} else {
				m.People = append(m.People, Ref{Name: part})
			}
		}
	}
	return m, true
}

func renderDay(d Day, loc *time.Location) string {
	var b strings.Builder
	b.WriteString("# " + d.Date.Format(dayLayout) + "\n\n")
	if s := strings.TrimSpace(d.Summary); s != "" {
		b.WriteString("## Summary\n\n" + s + "\n\n")
	}
	if len(d.Moments) > 0 {
		b.WriteString("## Moments\n\n")
		for _, m := range d.Moments {
			b.WriteString("- " + renderMoment(m, loc) + "\n")
		}
	}
	return b.String()
}

func renderMoment(m Moment, loc *time.Location) string {
	var tags []string
	if m.Channel != "" {
		tags = append(tags, "#"+oneLine(m.Channel))
	}
	for _, p := range m.People {
		name := strings.NewReplacer(";", ",", "[", "(", "]", ")").Replace(oneLine(p.Name))
		if p.ID != "" {
			tags = append(tags, name+":"+p.ID)
		} else if name != "" {
			tags = append(tags, name)
		}
	}
	line := m.At.In(loc).Format(clockLayout) + " "
	if len(tags) > 0 {
		line += "[" + strings.Join(tags, "; ") + "] "
	}
	return line + oneLine(m.Text)
}

// Recall picks the moments from the last days worth bringing back for what is
// happening now: those about the people present, those sharing words with the
// conversation, and those recent enough to still be on her mind.
//
// Keyword overlap rather than embeddings because an embedding is a model call
// per moment, and the relays this has to run on do not all offer one. It is
// crude and it is enough at this scale: what matters most is who was there.
// Moments at or after before are skipped — the caller already has those in
// front of her as the live transcript.
func (s *Store) Recall(guildID string, now, before time.Time, words, people []string, days, limit int) ([]Moment, error) {
	recent, err := s.Days(guildID, now, days)
	if err != nil {
		return nil, err
	}

	want := make(map[string]bool, len(words))
	for _, w := range words {
		want[w] = true
	}
	present := make(map[string]bool, len(people))
	for _, id := range people {
		present[id] = true
	}

	type scored struct {
		m     Moment
		score float64
	}
	var all []scored
	for _, day := range recent {
		for _, m := range day.Moments {
			if !m.At.Before(before) {
				continue
			}
			score := recency(now.Sub(m.At))
			for _, p := range m.People {
				if present[p.ID] {
					score += 1.0
					break
				}
			}
			if len(want) > 0 {
				hits := 0
				for _, w := range Keywords(m.Text) {
					if want[w] {
						hits++
					}
				}
				score += math.Min(1.2, 0.4*float64(hits))
			}
			all = append(all, scored{m, score})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]Moment, 0, len(all))
	for _, s := range all {
		out = append(out, s.m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// recency is how much a moment's age alone keeps it on her mind: all of it
// within the hour, half after a day and a half, little after a week.
func recency(age time.Duration) float64 {
	if age < time.Hour {
		return 1
	}
	return math.Exp(-age.Hours() / 52)
}

// Keywords are the words in text worth matching on: lowercased, four letters
// or more, so "the" and "you" never make two moments look related.
func Keywords(text string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, f := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(f)) < 4 || stopword[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// stopword are words long enough to pass the length filter and still say
// nothing about a subject, in the two languages she is spoken to in.
var stopword = map[string]bool{
	"that": true, "this": true, "with": true, "have": true, "what": true,
	"just": true, "like": true, "your": true, "they": true, "were": true,
	"been": true, "from": true, "about": true, "would": true, "there": true,
	"their": true, "when": true, "then": true, "than": true, "some": true,
	"said": true, "meant": true, "into": true, "will": true, "does": true,
	"это": true, "если": true, "чтобы": true, "когда": true, "тебя": true,
	"меня": true, "тебе": true, "мне": true, "просто": true, "только": true,
}
