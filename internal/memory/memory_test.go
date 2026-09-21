package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const guild = "111"

func open(t *testing.T) *Store {
	t.Helper()
	loc := time.FixedZone("MSK", 3*3600)
	s, err := Open(t.TempDir(), loc)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSelfRoundTrips(t *testing.T) {
	s := open(t)
	at := time.Date(2026, 9, 21, 14, 5, 0, 0, s.Location())
	err := s.UpdateSelf(guild, func(me *Self) {
		me.Lately = "Quiet week. Big M keeps trying to be my friend."
		me.Mood = "bored, a bit curious"
		me.MoodAt = at
	})
	if err != nil {
		t.Fatal(err)
	}
	me, err := s.Self(guild)
	if err != nil {
		t.Fatal(err)
	}
	if me.Lately != "Quiet week. Big M keeps trying to be my friend." || me.Mood != "bored, a bit curious" || !me.MoodAt.Equal(at) {
		t.Fatalf("self came back as %+v", me)
	}
}

func TestPersonRoundTripsAndKeepsNotesBounded(t *testing.T) {
	s := open(t)
	day := time.Date(2026, 9, 19, 0, 0, 0, 0, s.Location())
	for i := 0; i < MaxNotes+5; i++ {
		err := s.UpdatePerson(guild, "123", func(p *Person) {
			p.Name = "Big M"
			p.Who = "Runs the server. Builds things."
			p.Between = "He keeps pushing; I keep him at arm's length."
			p.Feeling = "wary"
			p.Notes = append(p.Notes, Note{Day: day, Text: "note " + string(rune('a'+i))})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	p, ok, err := s.Person(guild, "123")
	if err != nil || !ok {
		t.Fatalf("person not found: %v", err)
	}
	if p.Name != "Big M" || p.Who != "Runs the server. Builds things." || p.Feeling != "wary" ||
		p.Between != "He keeps pushing; I keep him at arm's length." {
		t.Fatalf("person came back as %+v", p)
	}
	if len(p.Notes) != MaxNotes {
		t.Fatalf("kept %d notes, want %d", len(p.Notes), MaxNotes)
	}
	if last := p.Notes[len(p.Notes)-1]; last.Text != "note "+string(rune('a'+MaxNotes+4)) || !last.Day.Equal(day) {
		t.Fatalf("newest note is %+v", last)
	}
}

func TestPersonRefusesPathLikeIDs(t *testing.T) {
	s := open(t)
	if err := s.UpdatePerson(guild, "../escape", func(*Person) {}); err == nil {
		t.Fatal("a path-like id was accepted")
	}
	if err := s.UpdateSelf("../..", func(*Self) {}); err == nil {
		t.Fatal("a path-like guild was accepted")
	}
}

func TestMomentsRoundTripThroughTheDayFile(t *testing.T) {
	s := open(t)
	at := time.Date(2026, 9, 19, 13, 34, 0, 0, s.Location())
	m := Moment{
		At:      at,
		Channel: "chat",
		People:  []Ref{{ID: "123", Name: "Big M"}},
		Text:    `said to Big M: "that's an interesting concept" — meant: the code city idea is clever`,
	}
	if err := s.AddMoment(guild, m); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSummary(guild, at, "He shared his project. I liked it."); err != nil {
		t.Fatal(err)
	}
	day, err := s.Day(guild, at)
	if err != nil {
		t.Fatal(err)
	}
	if day.Summary != "He shared his project. I liked it." || len(day.Moments) != 1 {
		t.Fatalf("day came back as %+v", day)
	}
	got := day.Moments[0]
	if !got.At.Equal(at) || got.Channel != "chat" || got.Text != m.Text ||
		len(got.People) != 1 || got.People[0] != m.People[0] {
		t.Fatalf("moment came back as %+v", got)
	}
}

func TestRecallPrefersWhoIsPresentAndSkipsTheLiveTranscript(t *testing.T) {
	s := open(t)
	now := time.Date(2026, 9, 21, 16, 0, 0, 0, s.Location())
	add := func(at time.Time, who, text string) {
		t.Helper()
		m := Moment{At: at, Text: text}
		if who != "" {
			m.People = []Ref{{ID: who, Name: who}}
		}
		if err := s.AddMoment(guild, m); err != nil {
			t.Fatal(err)
		}
	}
	add(now.Add(-50*time.Hour), "123", "he shared his code city project")
	add(now.Add(-49*time.Hour), "999", "someone else talked about cooking")
	add(now.Add(-48*time.Hour), "", "the room was quiet")
	add(now.Add(-2*time.Minute), "123", "he said hi just now")

	got, err := s.Recall(guild, now, now.Add(-10*time.Minute), Keywords("how is the city going"), []string{"123"}, 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "he shared his code city project" {
		t.Fatalf("recalled %+v", got)
	}
}

func TestThreadsReplaceByKeyAndClose(t *testing.T) {
	s := open(t)
	due := time.Date(2026, 9, 22, 18, 0, 0, 0, s.Location())
	th := Thread{Due: due, Person: Ref{ID: "123", Name: "Big M"}, Text: "ask how the naming went"}
	if err := s.AddThread(guild, th); err != nil {
		t.Fatal(err)
	}
	later := th
	later.Due = due.Add(24 * time.Hour)
	if err := s.AddThread(guild, later); err != nil {
		t.Fatal(err)
	}
	all, err := s.Threads(guild)
	if err != nil {
		t.Fatal(err)
	}
	if open := Unfinished(all); len(open) != 1 || !open[0].Due.Equal(later.Due) || open[0].Person != th.Person {
		t.Fatalf("threads are %+v", all)
	}
	if err := s.CloseThread(guild, th.Key()); err != nil {
		t.Fatal(err)
	}
	all, _ = s.Threads(guild)
	if len(Unfinished(all)) != 0 || len(all) != 1 || !all[0].Done {
		t.Fatalf("after closing, threads are %+v", all)
	}
}

func TestFilesAreReadableAndHandEditsSurvive(t *testing.T) {
	s := open(t)
	if err := s.UpdatePerson(guild, "123", func(p *Person) { p.Name = "Big M"; p.Who = "An admin." }); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root(), guild, peopleDir, "123.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "## Who they are\n\nAn admin.") {
		t.Fatalf("file reads:\n%s", raw)
	}

	// A person fixes her notes by hand, with CRLF line endings and an
	// undated note.
	edited := strings.ReplaceAll(string(raw), "An admin.", "An admin. Kind, underneath.") + "## Notes\n\n- likes GTA\n"
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(edited, "\n", "\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	p, _, err := s.Person(guild, "123")
	if err != nil {
		t.Fatal(err)
	}
	if p.Who != "An admin. Kind, underneath." || len(p.Notes) != 1 || p.Notes[0].Text != "likes GTA" {
		t.Fatalf("hand edit read back as %+v", p)
	}
}

func TestForgetMovesAside(t *testing.T) {
	s := open(t)
	if err := s.UpdateSelf(guild, func(me *Self) { me.Mood = "fine" }); err != nil {
		t.Fatal(err)
	}
	aside, err := s.Forget(guild, time.Now())
	if err != nil || aside == "" {
		t.Fatalf("forget: %q %v", aside, err)
	}
	if _, err := os.Stat(filepath.Join(aside, selfFile)); err != nil {
		t.Fatalf("nothing kept aside: %v", err)
	}
	me, _ := s.Self(guild)
	if me.Mood != "" {
		t.Fatal("she still remembers after forgetting")
	}
	if len(s.Guilds()) != 0 {
		t.Fatalf("guilds after forget: %v", s.Guilds())
	}
}
