package memory

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Thread is something she means to do: ask someone how a thing went, bring
// something up again, check on someone.
//
// These are what give her a reason to start a conversation that is her own
// and that she can name afterwards, rather than a timer firing on a feeling.
// See docs/persona.md.
type Thread struct {
	// Due is when it becomes worth doing; zero means whenever.
	Due time.Time
	// Person is who it is about, when it is about someone.
	Person Ref
	Text   string
	Done   bool
	// Source is what it came from: an intention is always her
	// interpretation of a moment, or of a day she reflected on.
	Source Source
}

// Key identifies a thread for closing it. The text and the person, since
// that is what a person editing the file would recognise as "the same one".
func (t Thread) Key() string {
	return t.Person.ID + "|" + strings.ToLower(oneLine(t.Text))
}

// threadLine reads "[ ] 2026-09-22 18:00 [Big M:123] text", with the date and
// the person both optional.
var threadLine = regexp.MustCompile(`^\[( |x|X)\]\s+(?:(\d{4}-\d{2}-\d{2} \d{1,2}:\d{2})\s+)?(?:\[([^\]]*)\]\s+)?(.*)$`)

// MaxOpenThreads is how many unfinished intentions she carries. Past it the
// oldest are let go, the way anyone forgets the fifth thing they meant to do.
const MaxOpenThreads = 12

// keepDone is how many finished threads stay in the file, for a person
// reading it to see what she followed up on.
const keepDone = 10

// Threads reads what she means to do in a guild, open ones first.
func (s *Store) Threads(guildID string) ([]Thread, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return readThreads(filepath.Join(dir, threadsFile), s.loc)
}

// Unfinished returns the threads not yet done, in file order.
func Unfinished(threads []Thread) []Thread {
	var out []Thread
	for _, t := range threads {
		if !t.Done {
			out = append(out, t)
		}
	}
	return out
}

// AddThread records a new intention. One that matches an open thread by Key
// replaces it, so being reminded of a plan moves it rather than doubling it.
func (s *Store) AddThread(guildID string, t Thread) error {
	if strings.TrimSpace(t.Text) == "" {
		return nil
	}
	return s.updateThreads(guildID, func(all []Thread) []Thread {
		kept := all[:0]
		for _, old := range all {
			if !old.Done && old.Key() == t.Key() {
				continue
			}
			kept = append(kept, old)
		}
		return append(kept, t)
	})
}

// CloseThread marks a thread done. It reports nothing when there is no such
// thread: closing something already gone is not an error.
func (s *Store) CloseThread(guildID, key string) error {
	return s.updateThreads(guildID, func(all []Thread) []Thread {
		for i := range all {
			if !all[i].Done && all[i].Key() == key {
				all[i].Done = true
				break
			}
		}
		return all
	})
}

func (s *Store) updateThreads(guildID string, change func([]Thread) []Thread) error {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(dir, threadsFile)
	all, err := readThreads(path, s.loc)
	if err != nil {
		return err
	}
	all = trimThreads(change(all))
	return writeFile(path, renderThreads(all, s.loc))
}

// trimThreads keeps the newest open threads and the last few done ones.
func trimThreads(all []Thread) []Thread {
	var open, done []Thread
	for _, t := range all {
		if t.Done {
			done = append(done, t)
		} else {
			open = append(open, t)
		}
	}
	if len(open) > MaxOpenThreads {
		open = open[len(open)-MaxOpenThreads:]
	}
	if len(done) > keepDone {
		done = done[len(done)-keepDone:]
	}
	return append(open, done...)
}

func readThreads(path string, loc *time.Location) ([]Thread, error) {
	text, err := readFile(path)
	if err != nil || text == "" {
		return nil, err
	}
	var out []Thread
	for _, item := range bullets(text) {
		match := threadLine.FindStringSubmatch(item)
		if match == nil {
			continue
		}
		text, src := splitSource(strings.TrimSpace(match[4]))
		t := Thread{Done: match[1] != " ", Text: text, Source: src}
		if match[2] != "" {
			t.Due, _ = time.ParseInLocation(minuteLayout, match[2], loc)
		}
		if ref := strings.TrimSpace(match[3]); ref != "" {
			if i := strings.LastIndex(ref, ":"); i > 0 {
				t.Person = Ref{Name: strings.TrimSpace(ref[:i]), ID: strings.TrimSpace(ref[i+1:])}
			} else {
				t.Person = Ref{Name: ref}
			}
		}
		out = append(out, t)
	}
	return out, nil
}

func renderThreads(all []Thread, loc *time.Location) string {
	sorted := append([]Thread(nil), all...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Done != sorted[j].Done {
			return !sorted[i].Done
		}
		return sorted[i].Due.Before(sorted[j].Due)
	})

	var b strings.Builder
	b.WriteString("# Things I mean to do\n\n")
	for _, t := range sorted {
		b.WriteString("- [")
		if t.Done {
			b.WriteString("x")
		} else {
			b.WriteString(" ")
		}
		b.WriteString("] ")
		if !t.Due.IsZero() {
			b.WriteString(t.Due.In(loc).Format(minuteLayout) + " ")
		}
		if t.Person.Name != "" || t.Person.ID != "" {
			name := refName(t.Person.Name)
			if t.Person.ID != "" {
				name += ":" + t.Person.ID
			}
			b.WriteString("[" + name + "] ")
		}
		b.WriteString(withSource(t.Text, t.Source) + "\n")
	}
	return b.String()
}

// CloseThreadsAbout marks every open intention about one person done, and
// reports how many. Asked to drop something, she drops the things she meant
// to do about them too: in production a dozen open "watch whether he…"
// intentions were what kept an evening's pressing going.
func (s *Store) CloseThreadsAbout(guildID, userID string) (int, error) {
	if userID == "" {
		return 0, nil
	}
	closed := 0
	err := s.updateThreads(guildID, func(all []Thread) []Thread {
		for i := range all {
			if !all[i].Done && all[i].Person.ID == userID {
				all[i].Done = true
				closed++
			}
		}
		return all
	})
	return closed, err
}
