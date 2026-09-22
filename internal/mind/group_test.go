package mind

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Behaviour in a group, after the first live log on a real server: nine
// follow-ups on one person, answered questions asked again, and a message to
// someone else taken as carrying on with her.

// bigM is a scene where Big M is talking to her.
func bigM() Scene {
	s := sceneWith(Turn{UserID: "365", Username: "Big M", Content: "just automated greetings - that's the whole functionality", At: noon})
	s.UserID, s.Username, s.MessageID = "365", "Big M", "m1"
	return s
}

func openThreads(t *testing.T, m *Mind) []memory.Thread {
	t.Helper()
	threads, err := m.Memory.Threads(guildID)
	if err != nil {
		t.Fatal(err)
	}
	return memory.Unfinished(threads)
}

// A follow-up she already means to is not added again, however it is worded;
// and past two for one person, the oldest gives way.
func TestOneIntentionIsKeptOnce(t *testing.T) {
	m, _ := newMind(t)
	s := bigM()
	for _, later := range []string{
		"check whether Big M actually explains what the bot does",
		"check whether Big M actually explains what the bot does this evening",
	} {
		if err := m.Absorb(s, Appraisal{Later: later}); err != nil {
			t.Fatal(err)
		}
	}
	if got := openThreads(t, m); len(got) != 1 {
		t.Fatalf("a repeat was kept: %+v", got)
	}
	for _, later := range []string{"ask how the role test went", "see whether Duchess got the link"} {
		if err := m.Absorb(s, Appraisal{Later: later}); err != nil {
			t.Fatal(err)
		}
	}
	got := openThreads(t, m)
	if len(got) != maxThreadsPer || got[0].Text != "ask how the role test went" {
		t.Errorf("open for Big M: %+v", got)
	}
}

// A moment that answers something she meant to follow up closes it there
// and then, by the number it was shown under.
func TestAnAnsweredQuestionIsSettled(t *testing.T) {
	m, p := newMind(t, `{"act":"reply","intent":"fair","settled":[1, 7]}`)
	s := bigM()
	if err := m.Absorb(s, Appraisal{Later: "find out what the link is actually about"}); err != nil {
		t.Fatal(err)
	}
	k, err := m.Know(s)
	if err != nil {
		t.Fatal(err)
	}
	a, err := m.Consider(context.Background(), s, k)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.sent[0][1].Content, "1. find out what the link is actually about") {
		t.Fatalf("the thread was not shown by number:\n%s", p.sent[0][1].Content)
	}
	if len(a.Settled) != 1 {
		t.Fatalf("settled %v, want only the one that exists", a.Settled)
	}
	if err := m.Absorb(s, a); err != nil {
		t.Fatal(err)
	}
	if got := openThreads(t, m); len(got) != 0 {
		t.Errorf("still open: %+v", got)
	}
}

// With others talking, a line right after hers is not stated as meant for
// her.
func TestAFollowUpInACrowdIsNotAssumedToBeHers(t *testing.T) {
	m, p := newMind(t, `{"act":"ignore"}`)
	s := bigM()
	s.Trigger, s.Crowd = TriggerFollowUp, true
	if _, err := m.Consider(context.Background(), s, Known{}); err != nil {
		t.Fatal(err)
	}
	prompt := p.sent[0][1].Content
	if strings.Contains(prompt, "carried on talking with you") || !strings.Contains(prompt, "may be to you or to someone else") {
		t.Errorf("what just happened:\n%s", prompt[strings.LastIndex(prompt, "What just happened"):])
	}
}

// The same feeling about the same person is kept once, whatever it is said
// to be about.
func TestTheSameFeelingIsKeptOnce(t *testing.T) {
	m, _ := newMind(t)
	m.Feelings = true
	s := bigM()
	for i, about := range []string{"Big M's soft no", "the whole exchange", "Big M laughing at himself"} {
		s.Now = noon.Add(time.Duration(i) * time.Minute)
		what := "mild amusement"
		if i == 2 {
			what = "mild amusement, a touch of warmth"
		}
		if err := m.Absorb(s, Appraisal{FeelingWhat: what, FeelingAbout: about, Weight: 0.3}); err != nil {
			t.Fatal(err)
		}
	}
	self, _ := m.Memory.Self(guildID)
	if len(self.Feelings) != 1 || self.Feelings[0].About != "Big M laughing at himself" {
		t.Errorf("feelings %+v", self.Feelings)
	}
}

// A garbled name of someone she knows is not kept in what she means to do;
// a name that resembles nobody, or the real one, is.
func TestAGarbledNameIsNotKept(t *testing.T) {
	s := bigM()
	for later, kept := range map[string]bool{
		"nudge Pewtato about the link they shared":  false,
		"ask Pewcifer what the link was about":      true,
		"see whether Duchess got the link sorted":   true,
		"Pewtato is not a name at the start either": true,
	} {
		m, _ := newMind(t)
		if err := m.Memory.UpdatePerson(guildID, "729", func(p *memory.Person) { p.Name = "Pewcifer⭐⭐" }); err != nil {
			t.Fatal(err)
		}
		if err := m.Absorb(s, Appraisal{Later: later}); err != nil {
			t.Fatal(err)
		}
		if got := len(openThreads(t, m)) == 1; got != kept {
			t.Errorf("%q kept: %v, want %v", later, got, kept)
		}
	}
}

// Woken early is a fact she is given, like the time she woke.
func TestSheIsToldSheWasWokenEarly(t *testing.T) {
	s := Scene{Now: noon, Woke: noon.Add(-10 * time.Minute)}
	if got := renderBody(s, "She", "has"); got != "She woke at 13:24." {
		t.Errorf("woke on her own: %q", got)
	}
	s.WokenEarly = true
	if got := renderBody(s, "She", "has"); got != "She was woken early, at 13:24." {
		t.Errorf("woken: %q", got)
	}
}
