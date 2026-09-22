package mind

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Step 2 of docs/persona-v3.md: provenance, identity, voice.

// What an appraisal says about someone rests on the message appraised, and
// the code stamps it: told is stated, made of is interpreted.
func TestAbsorbStampsWhereThingsCameFrom(t *testing.T) {
	m, _ := newMind(t)
	s := sceneWith(him("i named it cityscope", noon))
	s.MessageID = "4242"
	err := m.Absorb(s, Appraisal{Note: "named his project cityscope", Between: "warming up", Toward: "curious", Later: "ask how cityscope is going"})
	if err != nil {
		t.Fatal(err)
	}
	p, _, _ := m.Memory.Person(guildID, "123")
	if n := p.Notes[0]; n.Source.Kind != memory.Stated || n.Source.Ref != "msg 4242" {
		t.Errorf("note source %+v", n.Source)
	}
	if p.BetweenFrom.Kind != memory.Interpreted || p.FeelingFrom.Kind != memory.Interpreted {
		t.Errorf("between %+v, feeling %+v", p.BetweenFrom, p.FeelingFrom)
	}
	threads, _ := m.Memory.Threads(guildID)
	if threads[0].Source.Kind != memory.Interpreted {
		t.Errorf("thread source %+v", threads[0].Source)
	}
}

// A proposal about someone who did not speak in the scene is refused: the
// model does not get to write a dossier on someone who was not there.
func TestAbsorbRefusesNotesAboutSomeoneNotInTheScene(t *testing.T) {
	m, _ := newMind(t)
	s := sceneWith(Turn{UserID: "999", Username: "Big N", Content: "hi", At: noon})
	if err := m.Absorb(s, Appraisal{Note: "has a cat", Between: "cold", Later: "ask about the cat"}); err != nil {
		t.Fatal(err)
	}
	p, _, _ := m.Memory.Person(guildID, "123")
	if len(p.Notes) != 0 || p.Between != "" {
		t.Errorf("wrote about someone absent: %+v", p)
	}
	if threads, _ := m.Memory.Threads(guildID); len(threads) != 0 {
		t.Errorf("thread about someone absent: %+v", threads)
	}
}

// selfFactDay puts a day of her lines in memory, each a message of hers.
func selfFactDay(t *testing.T, m *Mind, lines ...string) time.Time {
	t.Helper()
	day := noon.Add(-24 * time.Hour)
	for i, line := range lines {
		s := sceneWith(him("so?", day))
		s.Now = day.Add(time.Duration(i) * time.Minute)
		if err := m.Said(s, Appraisal{Intent: "a private intention"}, line, "", "70000000000000000"+string(rune('1'+i))); err != nil {
			t.Fatal(err)
		}
	}
	return day
}

func TestReflectSelfKeepsWhatSheSaidAboutHerself(t *testing.T) {
	m, p := newMind(t, `{"facts":[{"text":"hates Rust — the compiler lectures her","line":1,"replaces":0,"contradicts":0}]}`)
	m.SelfFacts = true
	day := selfFactDay(t, m, "rust? no. the compiler lectures me like a hall monitor")

	did, err := m.ReflectSelf(context.Background(), guildID, day)
	if err != nil || !did {
		t.Fatalf("did %v, err %v", did, err)
	}
	me, _ := m.Memory.Me(guildID)
	if len(me.Facts) != 1 || me.Facts[0].Source.Kind != memory.Stated || me.Facts[0].Source.Ref != "msg 700000000000000001" {
		t.Fatalf("facts %+v", me.Facts)
	}
	if me.Through.IsZero() {
		t.Error("the day was not marked read")
	}
	// Her words, never what she meant, and never an id.
	prompt := p.sent[0][1].Content
	if strings.Contains(prompt, "a private intention") {
		t.Errorf("an intention reached the self-fact reading:\n%s", prompt)
	}
	if regexp.MustCompile(`\d{17,20}`).MatchString(prompt) {
		t.Errorf("an id reached the model:\n%s", prompt)
	}
}

func TestReflectSelfRefusesWhatCitesNothingOrRepeats(t *testing.T) {
	m, _ := newMind(t, `{"facts":[
		{"text":"invented out of nowhere","line":9},
		{"text":"hates Rust","line":1},
		{"text":"hates Rust, really","line":1}
	]}`)
	day := selfFactDay(t, m, "i hate rust")
	if _, err := m.ReflectSelf(context.Background(), guildID, day); err != nil {
		t.Fatal(err)
	}
	me, _ := m.Memory.Me(guildID)
	if len(me.Facts) != 1 || me.Facts[0].Text != "hates Rust" {
		t.Errorf("facts %+v", me.Facts)
	}
}

// Changing her mind supersedes the old fact; contradicting the card keeps
// both and flags the conflict for the author.
func TestReflectSelfSupersedesAndFlagsConflicts(t *testing.T) {
	m, _ := newMind(t, `{"facts":[
		{"text":"likes Rust now","line":1,"replaces":1},
		{"text":"has never read a novel","line":2,"contradicts":1}
	]}`)
	m.Character.Specifics = []string{"reads a novel a week"}
	if err := m.Memory.UpdateMe(guildID, func(me *memory.Me) {
		me.Facts = []memory.SelfFact{{Text: "hates Rust", Source: memory.Message(memory.Stated, "1")}}
	}); err != nil {
		t.Fatal(err)
	}
	day := selfFactDay(t, m, "ok rust grew on me", "never read a novel in my life")
	if _, err := m.ReflectSelf(context.Background(), guildID, day); err != nil {
		t.Fatal(err)
	}
	me, _ := m.Memory.Me(guildID)
	if len(me.Superseded) != 1 || me.Superseded[0].Text != "hates Rust" {
		t.Errorf("superseded %+v", me.Superseded)
	}
	if c := me.Conflicts(); len(c) != 1 || c[0].Conflict != "reads a novel a week" {
		t.Errorf("conflicts %+v", c)
	}
	if len(me.Facts) != 2 {
		t.Errorf("current facts %+v", me.Facts)
	}
}

func TestReflectSelfMakesNoCallOnADayWithNothingOfHers(t *testing.T) {
	m, p := newMind(t)
	day := noon.Add(-24 * time.Hour)
	if err := m.Memory.AddMoment(guildID, memory.Moment{At: day, Text: "someone said hi"}); err != nil {
		t.Fatal(err)
	}
	did, err := m.ReflectSelf(context.Background(), guildID, day)
	if did || err != nil || len(p.sent) != 0 {
		t.Fatalf("did %v, err %v, %d calls", did, err, len(p.sent))
	}
	if me, _ := m.Memory.Me(guildID); me.Through.IsZero() {
		t.Error("the day was not marked read")
	}
}

func TestSpecificsAreParsedApartFromThePersona(t *testing.T) {
	c, err := ParseCharacter("Domme", strings.NewReader("You are a fixture.\n\n## Specifics\n\n- hates Rust\n- reads a novel a week\n\n## Examples\n\nuser: hi\nher: no\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Specifics) != 2 || strings.Contains(c.Persona, "hates Rust") {
		t.Errorf("specifics %v, persona %q", c.Specifics, c.Persona)
	}
}

// Under budget every specific goes; over it, what the conversation touches
// comes first.
func TestSpecificsGiveWayToWhatTheConversationTouches(t *testing.T) {
	long := strings.Repeat("x", specificsBudget/2)
	all := []string{"likes trains " + long, "hates Rust because the compiler lectures " + long, "short"}
	got := pickSpecifics(all, []string{"rust", "compiler"})
	if len(got) != 2 || !strings.HasPrefix(got[0], "hates Rust") {
		t.Errorf("picked %v", got)
	}
	if got := pickSpecifics([]string{"a", "b"}, nil); len(got) != 2 {
		t.Errorf("under budget, picked %v", got)
	}
}

// What she has said about herself is shown only when it bears on the
// conversation: kept is not the same as in mind.
func TestSelfFactsAreShownOnlyWhenTheyBearOnIt(t *testing.T) {
	facts := []memory.SelfFact{{Text: "hates Rust"}, {Text: "sleeps badly"}, {Text: "hated mornings", Superseded: noon}}
	got := pickSelfFacts(facts, []string{"rust", "mornings"})
	if len(got) != 1 || got[0].Text != "hates Rust" {
		t.Errorf("shown %+v", got)
	}
}

// The voice gets the gist and the details, not the appraisal's reading.
func TestTheVoiceGetsDetailsButNotTheReading(t *testing.T) {
	m, p := newMind(t, "hm")
	m.Character.Specifics = []string{"hates Rust"}
	s := sceneWith(him("thoughts on rust?", noon))
	if err := m.Memory.UpdatePerson(guildID, "123", func(pp *memory.Person) {
		pp.Notes = []memory.Note{{Text: "is learning Rust"}}
	}); err != nil {
		t.Fatal(err)
	}
	k, _ := m.Know(s)
	a := Appraisal{Read: "SECRET READING", Feel: "SECRET FEELING", Intent: "tell him no"}
	if _, _, err := m.Speak(context.Background(), s, k, a, ""); err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, msg := range p.sent[0] {
		all.WriteString(msg.Content + "\n")
	}
	got := all.String()
	for _, banned := range []string{"SECRET READING", "SECRET FEELING"} {
		if strings.Contains(got, banned) {
			t.Errorf("the voice was given %q", banned)
		}
	}
	for _, want := range []string{"hates Rust", "is learning Rust", "Roughly what you want to get across: tell him no"} {
		if !strings.Contains(got, want) {
			t.Errorf("the voice was not given %q:\n%s", want, got)
		}
	}
}

func TestExamplesAreSampledInTheAuthorsOrder(t *testing.T) {
	m, _ := newMind(t)
	m.Character.Examples = nil
	for _, u := range []string{"a", "b", "c", "d", "e", "f"} {
		m.Character.Examples = append(m.Character.Examples, Exchange{User: u, Assistant: u})
	}
	m.ExamplesSample = 3
	rolls := []float64{0.9, 0.1, 0.5}
	m.Roll = func() float64 { r := rolls[0]; rolls = rolls[1:]; return r }
	got := m.sampleExamples()
	if len(got) != 3 {
		t.Fatalf("sampled %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].User >= got[i].User {
			t.Errorf("not in the author's order: %+v", got)
		}
	}
	m.ExamplesSample = 0
	if len(m.sampleExamples()) != 6 {
		t.Error("0 should show them all")
	}
}

// Over budget, recall gives way first and the person's notes last.
func TestFitTrimsRecallBeforeNotes(t *testing.T) {
	m, _ := newMind(t)
	k := Known{
		Recalled: []memory.Moment{{Text: "weak", Score: 0.1}, {Text: "strong", Score: 2}},
		People:   []memory.Person{{ID: "123", Notes: []memory.Note{{Text: "note"}}}},
	}
	size := func(k Known) int { return len(k.Recalled)*100 + len(k.People[0].Notes)*100 }
	got := m.fit(guildID, "test", k, 250, size)
	if len(got.Recalled) != 1 || got.Recalled[0].Text != "strong" || len(got.People[0].Notes) != 1 {
		t.Errorf("trimmed to %+v", got)
	}
	got = m.fit(guildID, "test", k, 50, size)
	if len(got.Recalled) != 0 || len(got.People[0].Notes) != 0 {
		t.Errorf("trimmed to %+v", got)
	}
	if len(k.People[0].Notes) != 1 {
		t.Error("fit changed the caller's dossier")
	}
}

// With drift, the weakest recalled slot sometimes goes to a loosely related
// moment below the cut; without it, recall is strictly by relevance.
func TestDriftBringsBackANeighbour(t *testing.T) {
	m, _ := newMind(t)
	s := sceneWith(him("thoughts on trains", noon))
	var pool []memory.Moment
	for i := 0; i < recallMoments; i++ {
		pool = append(pool, memory.Moment{At: noon.Add(-time.Duration(i+1) * time.Hour), Text: "relevant", Score: 5})
	}
	neighbour := memory.Moment{At: noon.Add(-48 * time.Hour), Text: "the night trains argument", Score: 0.1}
	stranger := memory.Moment{At: noon.Add(-49 * time.Hour), Text: "unrelated", Score: 0.1}
	pool = append(pool, neighbour, stranger)

	if got := m.drift(s, pool, nil); containsText(got, "the night trains argument") {
		t.Error("drifted with drift off")
	}
	m.Drift = 1
	m.Roll = func() float64 { return 0 }
	got := m.drift(s, pool, nil)
	if len(got) != recallMoments || !containsText(got, "the night trains argument") || containsText(got, "unrelated") {
		t.Errorf("recalled %+v", got)
	}
}

func containsText(ms []memory.Moment, text string) bool {
	for _, mo := range ms {
		if mo.Text == text {
			return true
		}
	}
	return false
}
