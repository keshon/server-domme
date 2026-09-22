package memory

import (
	"testing"
	"time"
)

func TestDerivedNeverGainsStrength(t *testing.T) {
	for _, c := range []struct {
		from []Kind
		want Kind
	}{
		{[]Kind{Observed}, Interpreted},
		{[]Kind{Authored, Stated}, Interpreted},
		{[]Kind{Interpreted, Observed}, Interpreted},
		{nil, Interpreted},
	} {
		if got := Derived(c.from...); got != c.want {
			t.Errorf("Derived(%v) = %s, want %s", c.from, got, c.want)
		}
	}
	if Interpreted.Outranks(Stated) || !Authored.Outranks(Observed) {
		t.Error("ranking is wrong")
	}
}

// Sources are written into the files and read back; a hand-written note
// without one must still read as a note.
func TestANoteKeepsItsSource(t *testing.T) {
	s, err := Open(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	err = s.UpdatePerson("1", "2", func(p *Person) {
		p.Notes = append(p.Notes, Note{Day: day, Text: "has a cat · a big one", Source: Message(Stated, "555")})
		p.Between, p.BetweenFrom = "warming", Message(Interpreted, "556")
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := s.Person("1", "2")
	if err != nil {
		t.Fatal(err)
	}
	n := p.Notes[0]
	if n.Text != "has a cat · a big one" || n.Source.Kind != Stated || n.Source.Ref != "msg 555" {
		t.Errorf("note read back as %+v", n)
	}
	if p.BetweenFrom.Kind != Interpreted {
		t.Errorf("between source %+v", p.BetweenFrom)
	}
	if got := parseNote("2026-09-22 written by hand", time.UTC); got.Text != "written by hand" || !got.Source.IsZero() {
		t.Errorf("hand-written note read as %+v", got)
	}
}

func TestSelfFactsRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	err = s.UpdateMe("1", func(m *Me) {
		m.Through = day
		m.Facts = append(m.Facts,
			SelfFact{Day: day, Text: "hates Rust", Source: Message(Stated, "10")},
			SelfFact{Day: day, Text: "Rust has grown on me", Source: Message(Stated, "11"), Conflict: "hates Rust because the compiler lectures her"})
		m.Superseded = append(m.Superseded,
			SelfFact{Day: day, Text: "likes mornings", Source: Message(Stated, "9"), Superseded: day})
	})
	if err != nil {
		t.Fatal(err)
	}
	me, err := s.Me("1")
	if err != nil {
		t.Fatal(err)
	}
	if !me.Through.Equal(day) || len(me.Facts) != 2 || len(me.Superseded) != 1 {
		t.Fatalf("read back %+v", me)
	}
	if c := me.Conflicts(); len(c) != 1 || c[0].Conflict != "hates Rust because the compiler lectures her" || c[0].Source.Ref != "msg 11" {
		t.Errorf("conflicts %+v", c)
	}
	if me.Superseded[0].Superseded.IsZero() {
		t.Errorf("superseded date lost: %+v", me.Superseded[0])
	}
}

func TestAMomentKeepsHerMessageAndItsKind(t *testing.T) {
	s, err := Open(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 22, 14, 5, 0, 0, time.UTC)
	for _, m := range []Moment{
		{At: at, Text: `said to Big M: "no"`, Said: "777"},
		{At: at, Text: "he finally named it", Kind: Interpreted},
	} {
		if err := s.AddMoment("1", m); err != nil {
			t.Fatal(err)
		}
	}
	d, _ := s.Day("1", at)
	if d.Moments[0].Said != "777" || d.Moments[1].Kind != Interpreted || len(d.Moments[0].People) != 0 {
		t.Errorf("moments read back as %+v", d.Moments)
	}
}
