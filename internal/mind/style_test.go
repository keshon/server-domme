package mind

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Examples are filed by the "###" headings under Examples; a heading that
// names no situation is reported and files nothing, and the phrases that
// are not hers are read as a list.
func TestExamplesAreFiledUnderSituations(t *testing.T) {
	c, err := ParseCharacter("Domme", strings.NewReader(`A fixture.

## Examples

> user: you up
> her: unfortunately

### jab
> user: your taste is awful
> her: and yet here you are

### question, compliment
> user: did you like it?
> her: i don't hand out compliments to be polite. yes

### nonsense
> user: x
> her: y

## Not her words

- the real question is
- Honestly
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Examples) != 4 {
		t.Fatalf("examples %+v", c.Examples)
	}
	if len(c.Examples[0].Situations) != 0 {
		t.Errorf("an example above every heading is filed: %+v", c.Examples[0])
	}
	if !c.Examples[1].fits(SituationJab) || c.Examples[1].fits(SituationQuestion) {
		t.Errorf("jab example: %+v", c.Examples[1])
	}
	if !c.Examples[2].fits(SituationQuestion) || !c.Examples[2].fits(SituationCompliment) {
		t.Errorf("two situations: %+v", c.Examples[2])
	}
	if len(c.UnknownSituations) != 1 || c.UnknownSituations[0] != "nonsense" {
		t.Errorf("unknown %v", c.UnknownSituations)
	}
	if len(c.NotHers) != 2 {
		t.Errorf("not hers %v", c.NotHers)
	}
	if strings.Contains(c.Persona, "jab") || strings.Contains(c.Persona, "real question") {
		t.Errorf("a subheading or a phrase leaked into the persona: %q", c.Persona)
	}
}

func TestTheAppraisalNamesTheSituation(t *testing.T) {
	for raw, want := range map[string]Situation{
		`{"act":"reply","intent":"x","situation":"Jab"}`:        SituationJab,
		`{"act":"reply","intent":"x","situation":"a vibe"}`:     "",
		`{"act":"reply","intent":"x","situation":"starting"}`:   SituationStarting,
		`{"act":"reply","intent":"x"}`:                          "",
		`{"act":"reply","intent":"x","situation":" apology  "}`: SituationApology,
	} {
		a, ok := parseAppraisal(raw)
		if !ok || a.Situation != want {
			t.Errorf("%s: %q", raw, a.Situation)
		}
	}
	m, _ := newMind(t)
	if !strings.Contains(m.appraisalShape(), `"situation"`) {
		t.Error("the appraisal is not asked for the situation")
	}
	m.Feelings = true
	if !strings.Contains(m.appraisalShape(), `"situation"`) {
		t.Error("with feelings, the appraisal is not asked for the situation")
	}
}

// The code names the situation where it knows it; otherwise it is the
// appraisal's.
func TestTheCodeNamesOnlyWhatItKnows(t *testing.T) {
	a := Appraisal{Situation: SituationJab}
	for trigger, want := range map[Trigger]Situation{
		TriggerMention:   SituationJab,
		TriggerOverheard: SituationJab,
		TriggerStart:     SituationStarting,
		TriggerReach:     SituationStarting,
		TriggerSight:     SituationStarting,
		TriggerLeave:     SituationLeaving,
	} {
		if got := situationOf(Scene{Trigger: trigger}, a); got != want {
			t.Errorf("%s: %q, want %q", trigger, got, want)
		}
	}
}

// With a situation, three in four of the sample are filed under it and come
// last, nearest the conversation; the rest come from elsewhere.
func TestTheSampleLeansOnTheSituation(t *testing.T) {
	m, _ := newMind(t)
	m.Character.Examples = nil
	for i, u := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		ex := Exchange{User: u, Assistant: u}
		if i%2 == 1 {
			ex.Situations = []Situation{SituationJab}
		}
		m.Character.Examples = append(m.Character.Examples, ex)
	}
	m.ExamplesSample = 4
	m.Roll = func() float64 { return 0.5 }
	got := m.sampleExamples(false, SituationJab)
	if len(got) != 4 {
		t.Fatalf("sampled %d", len(got))
	}
	if got[0].fits(SituationJab) {
		t.Errorf("the first is not from elsewhere: %+v", got)
	}
	for _, ex := range got[1:] {
		if !ex.fits(SituationJab) {
			t.Errorf("not the situation's: %+v", got)
		}
	}
	// A situation nothing is filed under samples from all of them.
	if got := m.sampleExamples(false, SituationApology); len(got) != 4 {
		t.Errorf("sampled %d with nothing filed", len(got))
	}
}

func TestTheStyleIsMeasuredFromHerExamples(t *testing.T) {
	c := &Character{
		Examples: []Exchange{{User: "x", Assistant: strings.Repeat("word ", 40)}, {User: "y", Assistant: "no"}},
		NotHers:  []string{"The Real Question Is", "honestly"},
	}
	st := c.Style()
	if st.MaxWords != 60 {
		t.Errorf("max words %d, want her longest stretched", st.MaxWords)
	}
	if got := (&Character{Examples: []Exchange{{User: "x", Assistant: "no"}}}).Style().MaxWords; got != minStyleWords {
		t.Errorf("a card of one-word examples allows %d words", got)
	}
	if o := st.check("ok but the real question is who"); o.Phrase != "the real question is" {
		t.Errorf("phrase %q", o.Phrase)
	}
	if o := st.check("that was dishonestly done"); o.Phrase != "" {
		t.Errorf("matched inside a word: %q", o.Phrase)
	}
	if o := st.check("Honestly? no"); o.Phrase != "honestly" {
		t.Errorf("case: %q", o.Phrase)
	}
}

// Too long is cut at a sentence boundary; a first sentence already too long
// is not cut, and is what counts as a miss.
func TestALongReplyIsCutAtASentence(t *testing.T) {
	st := Style{MaxWords: 6}
	if got := st.cut("one two three. four five six. seven eight nine"); got != "one two three. four five six." {
		t.Errorf("cut to %q", got)
	}
	if got := st.cut("one two three\n\nfour five six seven"); got != "one two three" {
		t.Errorf("two messages cut to %q", got)
	}
	long := "one two three four five six seven, eight. nine"
	if got := st.cut(long); got != long {
		t.Errorf("cut inside the first sentence: %q", got)
	}
	if !st.check(long).Long || st.check("one two three. four five six. seven eight nine").Long {
		t.Error("long is what a cut cannot fix")
	}
}

// A reply with words that are not hers is asked for again once, and the
// cleaner one is sent.
func TestSpeakAsksAgainForWordsThatAreNotHers(t *testing.T) {
	m, p := newMind(t, "the real question is whether you show up", "show up with your skyline then")
	m.StyleCheck = true
	m.Character.NotHers = []string{"the real question is"}
	s := sceneWith(Turn{UserID: "1", Username: "Big M", Content: "impressive", At: noon})
	got, _, err := m.Speak(context.Background(), s, Known{}, Appraisal{Act: ActReply, Intent: "dare him"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "show up with your skyline then" {
		t.Errorf("sent %q", got)
	}
	if len(p.sent) != 2 || !strings.Contains(p.sent[1][len(p.sent[1])-1].Content, "the real question is") {
		t.Error("the retry was not told what was wrong")
	}

	// Off, nothing is checked.
	m, p = newMind(t, "the real question is whether you show up")
	m.Character.NotHers = []string{"the real question is"}
	if _, _, err := m.Speak(context.Background(), s, Known{}, Appraisal{Act: ActReply}, ""); err != nil || len(p.sent) != 1 {
		t.Errorf("checked with the style check off: %v, %d calls", err, len(p.sent))
	}
}

// A reply that runs long but can be cut is cut, without another call.
func TestSpeakCutsALongReplyWithoutAskingAgain(t *testing.T) {
	long := "alright. " + strings.Repeat("this goes on and on. ", 20)
	m, p := newMind(t, long)
	m.StyleCheck = true
	s := sceneWith(Turn{UserID: "1", Username: "Big M", Content: "tell me", At: noon})
	got, _, err := m.Speak(context.Background(), s, Known{}, Appraisal{Act: ActReply}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.sent) != 1 {
		t.Errorf("%d calls for a reply a cut could fix", len(p.sent))
	}
	if n := len(strings.Fields(got)); n > m.Character.Style().MaxWords {
		t.Errorf("sent %d words", n)
	}
}

// What she said on her own is not something that happened around her: her
// life and her impulses cannot rest on it. What someone said to her can.
func TestHerOwnWordsAreNotEvidence(t *testing.T) {
	own := memory.Moment{Said: "m1", Text: `I spoke up on my own: "i'm building a city generator"`}
	if _, ok := observedPart(own); ok {
		t.Error("a line she spoke up with counts as observed")
	}
	answer := memory.Moment{Said: "m2", Text: `Big M: "tell me about it"` + saidArrow + `"i'm building a city generator"`}
	if text, ok := observedPart(answer); !ok || strings.Contains(text, "city generator") || !strings.Contains(text, "tell me about it") {
		t.Errorf("answer: %q, %v", text, ok)
	}
	walk := memory.Moment{Text: "passed through #art: dragons", Walk: true}
	if text, ok := observedPart(walk); !ok || text != walk.Text {
		t.Errorf("walk: %q", text)
	}
}

func TestLifeIsNotShownHerOwnWords(t *testing.T) {
	m, p := newMind(t, `{"life":[],"wants":[]}`)
	day := noon.Add(-24 * time.Hour)
	for _, mo := range []memory.Moment{
		{At: day.Add(time.Hour), Said: "m1", Text: `I spoke up on my own: "i'm building a city generator"`},
		{At: day.Add(2 * time.Hour), Said: "m2", Text: `Big M: "what are you up to"` + saidArrow + `"building a city generator"`},
	} {
		if err := m.Memory.AddMoment(guildID, mo); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.ReflectLife(context.Background(), guildID, day, noon); err != nil {
		t.Fatal(err)
	}
	shown := p.sent[0][1].Content
	if strings.Contains(shown, "city generator") {
		t.Errorf("her own claim was shown as something that happened:\n%s", shown)
	}
	if !strings.Contains(shown, "what are you up to") {
		t.Errorf("what Big M said was not shown:\n%s", shown)
	}
}

// An impulse must come from something she was shown, by number, and carries
// it; one from nothing, or from a number that is not there, is refused.
func TestAnImpulseComesFromSomethingShown(t *testing.T) {
	day := noon.Add(-time.Hour)
	seed := func(m *Mind) {
		for _, mo := range []memory.Moment{
			{At: day, Said: "m1", Text: `I spoke up on my own: "been sitting on a thing since last week"`},
			{At: day.Add(time.Minute), Text: `Rook: "finally named the game"`},
		} {
			if err := m.Memory.AddMoment(guildID, mo); err != nil {
				t.Fatal(err)
			}
		}
	}
	in := Idle{GuildID: guildID, Now: noon, People: []Candidate{{ID: "5", Name: "Rook", Here: true}}}

	m, p := newMind(t, `{"on_mind":"x","impulse":{"from":1,"to":"Rook","about":"ask what he named it"}}`)
	seed(m)
	res, err := m.IdleThink(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Impulse == nil || !strings.Contains(res.Impulse.From, "finally named the game") {
		t.Fatalf("impulse %+v", res.Impulse)
	}
	if shown := p.sent[0][1].Content; strings.Contains(shown, "sitting on a thing") {
		t.Errorf("her own line was offered as something to go on:\n%s", shown)
	}

	for _, reply := range []string{
		`{"on_mind":"x","impulse":{"to":"Rook","about":"that thing i've been sitting on"}}`,
		`{"on_mind":"x","impulse":{"from":7,"to":"Rook","about":"that thing"}}`,
	} {
		m, _ := newMind(t, reply)
		seed(m)
		if res, err := m.IdleThink(context.Background(), in); err != nil || res.Impulse != nil {
			t.Errorf("%s: impulse %+v, %v", reply, res.Impulse, err)
		}
	}
}

// A third question in a row is asked for again; if the retry is a question
// too, the question is cut when something comes before it.
func TestAThirdQuestionInARowIsNotSent(t *testing.T) {
	asking := sceneWith(
		Turn{UserID: "1", Username: "Big M", Content: "i built a music bot", At: noon},
		Turn{FromBot: true, Content: "what's the hardest part?", At: noon},
		Turn{UserID: "1", Username: "Big M", Content: "network hiccups", At: noon},
		Turn{FromBot: true, Content: "how do you handle them?", At: noon},
		Turn{UserID: "1", Username: "Big M", Content: "a chain of parsers", At: noon},
	)
	m, p := newMind(t, "smart. how do you balance speed and reliability?", "a chain is how i'd do it too")
	m.StyleCheck = true
	got, _, err := m.Speak(context.Background(), asking, Known{}, Appraisal{Act: ActReply}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a chain is how i'd do it too" {
		t.Errorf("sent %q", got)
	}
	if !strings.Contains(p.sent[1][len(p.sent[1])-1].Content, "ends in a question") {
		t.Error("the retry was not told what was wrong")
	}

	// Both a question: the shorter is kept, and its question cut.
	m, _ = newMind(t, "smart. how do you balance it?", "that's a sound approach. do you retry first?")
	m.StyleCheck = true
	if got, _, _ := m.Speak(context.Background(), asking, Known{}, Appraisal{Act: ActReply}, ""); got != "smart." {
		t.Errorf("a question after two was sent: %q", got)
	}

	// Two questions in a row are fine; so is a question after an answer.
	m, p = newMind(t, "how many parsers?")
	m.StyleCheck = true
	once := sceneWith(Turn{FromBot: true, Content: "what's the hardest part?", At: noon}, Turn{UserID: "1", Username: "Big M", Content: "network", At: noon})
	if got, _, _ := m.Speak(context.Background(), once, Known{}, Appraisal{Act: ActReply}, ""); got != "how many parsers?" || len(p.sent) != 1 {
		t.Errorf("a second question was checked: %q after %d calls", got, len(p.sent))
	}
}

func TestDropQuestionsKeepsWhatCameBefore(t *testing.T) {
	for in, want := range map[string]string{
		"that makes sense. how do you handle it? do you ever break it?": "that makes sense.",
		"smart.\n\nwhat's your fallback?":                               "smart.",
		"how do you handle it?":                                         "how do you handle it?",
		"fair. i'd do the same":                                         "fair. i'd do the same",
	} {
		if got := dropQuestions(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// Saying she is building something does not make her be building it.
func TestAClaimOfDoingIsNotASelfFact(t *testing.T) {
	m, _ := newMind(t, `{"facts":[
		{"text":"is building a city generator","line":1,"doing":true},
		{"text":"hates mornings","line":2}
	]}`)
	day := noon.Add(-24 * time.Hour)
	for i, text := range []string{`I spoke up on my own: "i'm building a city generator"`, `Big M: "morning"` + saidArrow + `"i hate mornings"`} {
		if err := m.Memory.AddMoment(guildID, memory.Moment{At: day.Add(time.Duration(i) * time.Minute), Said: fmt.Sprintf("m%d", i), Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.ReflectSelf(context.Background(), guildID, day); err != nil {
		t.Fatal(err)
	}
	me, _ := m.Memory.Me(guildID)
	if len(me.Facts) != 1 || me.Facts[0].Text != "hates mornings" {
		t.Errorf("facts %+v", me.Facts)
	}
}
