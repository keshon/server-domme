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

// Someone's line from a memory she was shown is not hers to send back: the
// production case, Big M's own words returned to him ninety minutes later.
func TestALineFromMemoryIsNotEchoed(t *testing.T) {
	m, _ := newMind(t, "you are very kind... Server Domme _cough-cough_")
	s := bigM()
	k := Known{Recalled: []memory.Moment{{
		At: noon.Add(-90 * time.Minute), Said: "m0", Weight: 0.3,
		Text: `Big M: "you are very kind... Server Domme _cough-cough_"` + saidArrow + `"i'd ask what pronoun you used"`,
	}}}
	if _, _, err := m.Speak(context.Background(), s, k, Appraisal{Act: ActReply}, ""); err != ErrEcho {
		t.Errorf("sent his line back: %v", err)
	}
	// Her own half of a memory is hers; repeating it is a different check.
	m, _ = newMind(t, "i'd ask what pronoun you used")
	if _, _, err := m.Speak(context.Background(), s, k, Appraisal{Act: ActReply}, ""); err != nil {
		t.Errorf("her own words from a memory counted as an echo: %v", err)
	}
}

// What the thinking hands the voice is asked for in her own first person:
// told "signal she's comfortable", the voice answered "she's doing fine" as
// if about someone else.
func TestTheGistIsAskedForInTheFirstPerson(t *testing.T) {
	m, _ := newMind(t)
	for _, feelings := range []bool{false, true} {
		m.Feelings = feelings
		shape := m.appraisalShape()
		for _, key := range []string{`"intent"`, `"then"`} {
			line := shape[strings.Index(shape, key):]
			line = line[:strings.Index(line, "\n")]
			if !strings.Contains(line, "first person") {
				t.Errorf("feelings %v: %s", feelings, line)
			}
		}
	}
}

// A life item carried from day to day lists each moment behind it once.
func TestALifeItemKeepsEachSourceOnce(t *testing.T) {
	m, _ := newMind(t, `{"life":[{"text":"Big M is deep in the parsing","moments":[1,2],"keeps":1}],"wants":[]}`)
	day := noon.Add(-24 * time.Hour)
	if err := m.Memory.UpdateSelf(guildID, func(me *memory.Self) {
		me.Life = []memory.LifeItem{{Text: "Big M is deep in the parsing", Since: day, Advanced: day,
			Sources: []string{"2026-09-18 12:46", "2026-09-18 16:06"}}}
	}); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{day.Add(time.Hour), day.Add(2 * time.Hour)} {
		if err := m.Memory.AddMoment(guildID, memory.Moment{At: at, Text: `Big M: "still on the threads"`}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.ReflectLife(context.Background(), guildID, day, noon); err != nil {
		t.Fatal(err)
	}
	self, _ := m.Memory.Self(guildID)
	if len(self.Life) != 1 {
		t.Fatalf("life %+v", self.Life)
	}
	seen := map[string]int{}
	for _, s := range self.Life[0].Sources {
		seen[s]++
	}
	for src, n := range seen {
		if n != 1 {
			t.Errorf("%s listed %d times: %v", src, n, self.Life[0].Sources)
		}
	}
	if len(seen) != 4 {
		t.Errorf("sources %v", self.Life[0].Sources)
	}
}

// Nobody's gender is guessed: she misgendered someone on a server whose
// name says nothing about who is on it.
func TestNobodysGenderIsGuessed(t *testing.T) {
	for _, rules := range []string{thinkingRules, reflectRules} {
		if !strings.Contains(rules, "gender") {
			t.Errorf("no rule about it in:\n%s", rules)
		}
	}
	if !strings.Contains(reflectRules, "no work or project of her own") {
		t.Error("her account of herself may still invent a project")
	}
}

// What she promises in an afterthought is hers to keep: "watch what happens
// at midnight" went out as a second message and was recorded nowhere,
// because only the first reply passes through the part of her that plans.
func TestAPromiseInAnAfterthoughtIsAskedFor(t *testing.T) {
	m, _ := newMind(t)
	shape := m.appraisalShape()
	line := shape[strings.Index(shape, `"later"`):]
	line = line[:strings.Index(line, "\n")]
	for _, want := range []string{"the afterthought", "a test she sets"} {
		if !strings.Contains(line, want) {
			t.Errorf("%q missing from: %s", want, line)
		}
	}
}

// A compact portrait: pronouns only as they said them, and what works with
// them learned from how it went. Both reach her thinking and her voice, and
// both survive a round trip through the file.
func TestAPersonsPortraitIsKept(t *testing.T) {
	m, _ := newMind(t)
	s := bigM()
	if err := m.Absorb(s, Appraisal{Pronouns: "he/him", Toward: "warming", Weight: 0.3}); err != nil {
		t.Fatal(err)
	}
	if err := m.Memory.UpdatePerson(guildID, "365", func(p *memory.Person) {
		p.Name, p.Works = "Big M", "he answers straight questions straight; small talk dies with him"
	}); err != nil {
		t.Fatal(err)
	}
	p, ok, err := m.Memory.Person(guildID, "365")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if p.Pronouns != "he/him" || p.PronounsFrom.Kind != memory.Stated {
		t.Errorf("pronouns %q from %v", p.Pronouns, p.PronounsFrom)
	}
	shown := renderPerson(p, "", noon)
	if !strings.Contains(shown, "(he/him)") || !strings.Contains(shown, "What works with them: he answers straight") {
		t.Errorf("her thinking is shown:\n%s", shown)
	}
	k := Known{People: []memory.Person{p}}
	s.UserID = "365"
	voice := m.voiceSystem(s, k)
	if !strings.Contains(voice, "(he/him)") || !strings.Contains(voice, "What works with them: he answers straight") {
		t.Errorf("her voice is shown:\n%s", voice)
	}
	// Nothing is guessed: no pronouns stated, nothing written.
	if err := m.Absorb(s, Appraisal{Toward: "warm"}); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := m.Memory.Person(guildID, "365"); p.Pronouns != "he/him" {
		t.Errorf("pronouns became %q", p.Pronouns)
	}
}

// The budgets are the deployment's when it sets them, the defaults
// otherwise.
func TestTheBudgetsAreTheDeploymentsToSet(t *testing.T) {
	m, _ := newMind(t)
	if m.thinkBudget() != defaultThinkBudget || m.voiceBudget() != defaultVoiceBudget {
		t.Errorf("defaults: %d, %d", m.thinkBudget(), m.voiceBudget())
	}
	m.ThinkBudget, m.VoiceBudget = 40000, 30000
	if m.thinkBudget() != 40000 || m.voiceBudget() != 30000 {
		t.Errorf("set: %d, %d", m.thinkBudget(), m.voiceBudget())
	}
}
