package mind

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
)

// scripted answers each call with the next reply in line, and keeps what it
// was sent.
type scripted struct {
	replies []string
	sent    [][]ai.Message
}

func (p *scripted) Generate(_ context.Context, msgs []ai.Message) (string, error) {
	p.sent = append(p.sent, msgs)
	if len(p.replies) == 0 {
		return "", ai.ErrNoBackend
	}
	r := p.replies[0]
	p.replies = p.replies[1:]
	return r, nil
}

const guildID = "900"

func newMind(t *testing.T, replies ...string) (*Mind, *scripted) {
	t.Helper()
	store, err := memory.Open(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	p := &scripted{replies: replies}
	c := &Character{
		Name:     "Domme",
		Persona:  "You have been on this server since it was three channels and an argument.",
		Lately:   "Quiet stretch. Nobody new.",
		Examples: []Exchange{{User: "you up", Assistant: "unfortunately"}},
	}
	return &Mind{Character: c, Provider: p, Memory: store}, p
}

var noon = time.Date(2026, 9, 19, 13, 34, 0, 0, time.UTC)

func sceneWith(lines ...Turn) Scene {
	return Scene{
		GuildID: guildID, GuildName: "Test", ChannelID: "c1", ChannelName: "chat",
		SelfName: "Domme", Now: noon, Trigger: TriggerMention,
		UserID: "123", Username: "Big M", Turns: lines,
	}
}

func him(text string, at time.Time) Turn {
	return Turn{UserID: "123", Username: "Big M", Content: text, At: at}
}

func her(text string, at time.Time) Turn {
	return Turn{FromBot: true, Content: text, At: at}
}

func TestConsiderReadsAFencedAppraisal(t *testing.T) {
	m, p := newMind(t, "sure, here you go:\n```json\n"+`{
  "read": "he is proud of his project and wants me to care",
  "feel": "mildly impressed",
  "toward": "warming to him",
  "mood": "awake",
  "act": "reply",
  "intent": "tell him the city idea is actually clever, ask what he calls it",
  "note": "building a code-city visualiser",
  "between": "",
  "remember": "",
  "later": "ask whether he found a name for it",
  "later_hours": "24"
}`+"\n```")
	s := sceneWith(him("an app that shows a codebase as a city", noon))
	k, err := m.Know(s)
	if err != nil {
		t.Fatal(err)
	}
	a, err := m.Consider(context.Background(), s, k)
	if err != nil {
		t.Fatal(err)
	}
	if a.Act != ActReply || a.Intent == "" || a.Later == "" || a.LaterHours != 24 || a.Toward != "warming to him" {
		t.Fatalf("appraisal read as %+v", a)
	}

	// Her own lines are marked as hers in what the thinker is shown.
	prompt := p.sent[0][1].Content
	if !strings.Contains(prompt, "Big M tagged you") {
		t.Errorf("the trigger is not stated:\n%s", prompt)
	}
}

func TestConsiderRejectsWhatIsNotAnAppraisal(t *testing.T) {
	m, _ := newMind(t, "i think she would just say hi")
	s := sceneWith(him("hi", noon))
	k, _ := m.Know(s)
	if _, err := m.Consider(context.Background(), s, k); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("got %v, want ErrUnreadable", err)
	}
}

func TestAParsedReactNeedsARealEmoji(t *testing.T) {
	a, _ := parseAppraisal(`{"act":"react","emoji":":salute:"}`)
	if a.Act != ActIgnore {
		t.Errorf("a custom emoji name stayed a reaction: %+v", a)
	}
	a, _ = parseAppraisal(`{"act":"react","emoji":"🫡"}`)
	if a.Act != ActReact {
		t.Errorf("a unicode emoji was refused: %+v", a)
	}
}

// The failure that retired v1: she said something supportive in the
// afternoon, and in the evening read it back as someone else's words. What
// she says is written down with what she meant, and it comes back to her.
func TestWhatSheSaidComesBackToHerAsHers(t *testing.T) {
	m, p := newMind(t, `{"act":"reply","read":"asking if I meant it","intent":"yes, I meant it"}`, "yes. still think it's clever")
	s := sceneWith(him("an app that shows a codebase as a city, what do you think?", noon))
	a := Appraisal{Act: ActReply, Intent: "tell him the idea is genuinely clever"}
	if err := m.Said(s, a, "that's a clever one, honestly", ""); err != nil {
		t.Fatal(err)
	}

	evening := noon.Add(7 * time.Hour)
	later := Scene{
		GuildID: guildID, ChannelName: "chat", SelfName: "Domme", Now: evening,
		Trigger: TriggerMention, UserID: "123", Username: "Big M",
		Turns: []Turn{him("did you mean what you said about my project?", evening)},
	}
	k, err := m.Know(later)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Consider(context.Background(), later, k); err != nil {
		t.Fatal(err)
	}
	prompt := p.sent[0][1].Content
	for _, want := range []string{"that's a clever one, honestly", "meant: tell him the idea is genuinely clever"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the evening prompt does not carry %q:\n%s", want, prompt)
		}
	}
}

func TestAbsorbWritesTheDossierTheMoodAndTheThread(t *testing.T) {
	m, _ := newMind(t)
	s := sceneWith(him("I'm testing it on a 1.5 million line repo", noon))
	a := Appraisal{
		Mood: "curious", Toward: "warming to him", Note: "tests on a 1.5M line repo at work",
		Between: "he is sincere; I am letting him in a little", Later: "ask how the big repo went", LaterHours: 30,
	}
	if err := m.Absorb(s, a); err != nil {
		t.Fatal(err)
	}
	// The same note twice is kept once.
	if err := m.Absorb(s, Appraisal{Note: "tests on a 1.5M line repo at work"}); err != nil {
		t.Fatal(err)
	}

	p, ok, _ := m.Memory.Person(guildID, "123")
	if !ok || p.Name != "Big M" || p.Feeling != "warming to him" || len(p.Notes) != 1 ||
		p.Between != "he is sincere; I am letting him in a little" || !p.LastTalked.Equal(noon) {
		t.Fatalf("dossier is %+v", p)
	}
	me, _ := m.Memory.Self(guildID)
	if me.Mood != "curious" || me.Lately != "Quiet stretch. Nobody new." {
		t.Fatalf("self is %+v", me)
	}
	threads, _ := m.Memory.Threads(guildID)
	if len(threads) != 1 || !threads[0].Due.Equal(noon.Add(30*time.Hour)) || threads[0].Person.ID != "123" {
		t.Fatalf("threads are %+v", threads)
	}
}

func TestSpeakNeverReturnsAControlWord(t *testing.T) {
	for _, reply := range []string{"SKIP", "skip.", `{"act":"ignore"}`, "   "} {
		m, _ := newMind(t, reply)
		s := sceneWith(him("explain this to me", noon))
		k, _ := m.Know(s)
		if got, _, err := m.Speak(context.Background(), s, k, Appraisal{Act: ActReply}, ""); !errors.Is(err, ErrControl) {
			t.Errorf("%q came back as %q, %v", reply, got, err)
		}
	}
	if IsControl("skip my ass") || IsControl("nothing to add, go on") {
		t.Error("a real line was taken for a control word")
	}
}

func TestSpeakRetriesARepeatOnceThenGivesUp(t *testing.T) {
	turns := []Turn{
		him("morning", noon),
		her("morning. keep it professional, or we'll revisit this", noon),
		him("I learned my lesson", noon),
		her("morning. let's see if you can keep it that way", noon),
		him("I will do my best", noon),
	}
	m, p := newMind(t, "morning. keep it professional and we'll get along", "ha. we'll see")
	s := sceneWith(turns...)
	k, _ := m.Know(s)
	got, _, err := m.Speak(context.Background(), s, k, Appraisal{Act: ActReply}, "")
	if err != nil || got != "ha. we'll see" {
		t.Fatalf("got %q, %v", got, err)
	}
	if last := p.sent[1][len(p.sent[1])-1].Content; !strings.Contains(last, "opens the same way") {
		t.Errorf("the retry was not told what to avoid: %q", last)
	}

	m, _ = newMind(t, "morning. again", "morning. and again")
	s = sceneWith(turns...)
	if _, _, err := m.Speak(context.Background(), s, k, Appraisal{Act: ActReply}, ""); !errors.Is(err, ErrRepeat) {
		t.Fatalf("a second repeat got %v", err)
	}
}

func TestVoiceStatesTheDecisionAfterTheTranscript(t *testing.T) {
	m, p := newMind(t, "clever, actually")
	s := sceneWith(him("what do you think of my code city?", noon))
	k, _ := m.Know(s)
	if _, _, err := m.Speak(context.Background(), s, k, Appraisal{Act: ActReply, Intent: "say it is clever"}, ""); err != nil {
		t.Fatal(err)
	}
	msgs := p.sent[0]
	last := msgs[len(msgs)-1]
	if last.Role != ai.RoleSystem || !strings.Contains(last.Content, "say it is clever") {
		t.Fatalf("the decision is not last: %+v", last)
	}
	if msgs[1].Content != "you up" || msgs[2].Content != "unfortunately" {
		t.Errorf("the examples are not replayed as turns: %+v", msgs[1:3])
	}
}

func TestReflectRewritesOnlyThePeopleInTheDay(t *testing.T) {
	m, p := newMind(t, `{
  "summary": "Big M shared his project. I was kinder than I planned.",
  "lately": "Something to talk about, for once.",
  "people": [
    {"person": 1, "who": "Runs the server; builds odd tools.", "between": "Warming up.", "feeling": "fond, a little",
     "stays": ["he showed me the code city before anyone else"]},
    {"person": 2, "who": "invented", "between": "invented", "feeling": "invented"}
  ],
  "done": [1],
  "threads": [{"text": "ask what he named it", "person": 1, "hours": 20}]
}`)
	if err := m.Memory.AddThread(guildID, memory.Thread{Text: "say hi to the new one"}); err != nil {
		t.Fatal(err)
	}
	s := sceneWith(him("code city", noon))
	if err := m.Said(s, Appraisal{Intent: "encourage him", Weight: 0.8}, "that's clever", ""); err != nil {
		t.Fatal(err)
	}

	did, err := m.Reflect(context.Background(), guildID, "Test", noon, noon.Add(15*time.Hour))
	if err != nil || !did {
		t.Fatalf("reflect: %v %v", did, err)
	}
	prompt := p.sent[0][1].Content
	if strings.Contains(prompt, "123") || !strings.Contains(prompt, "Person 1. ### Big M") || !strings.Contains(prompt, "it stayed with her") {
		t.Errorf("the reflection prompt:\n%s", prompt)
	}
	day, _ := m.Memory.Day(guildID, noon)
	if day.Summary != "Big M shared his project. I was kinder than I planned." {
		t.Errorf("summary is %q", day.Summary)
	}
	me, _ := m.Memory.Self(guildID)
	if me.Lately != "Something to talk about, for once." {
		t.Errorf("lately is %q", me.Lately)
	}
	his, _, _ := m.Memory.Person(guildID, "123")
	if his.Who != "Runs the server; builds odd tools." || his.Feeling != "fond, a little" ||
		len(his.Kept) != 1 || his.Kept[0].Text != "he showed me the code city before anyone else" {
		t.Errorf("his dossier is %+v", his)
	}
	people, _ := m.Memory.People(guildID)
	if len(people) != 1 {
		t.Errorf("a person who was not in the day got a dossier: %+v", people)
	}
	threads, _ := m.Memory.Threads(guildID)
	open := memory.Unfinished(threads)
	if len(open) != 1 || open[0].Text != "ask what he named it" || open[0].Person.ID != "123" {
		t.Errorf("open threads are %+v", open)
	}
}

// A Discord id is a stable identifier for a real account; no prompt carries
// one, however much of her memory is in it.
func TestNoPromptCarriesAnID(t *testing.T) {
	const id = "365177820663513089"
	m, p := newMind(t,
		`{"act":"reply","intent":"hi","weight":0.9,"later":"ask him","note":"has a cat"}`,
		"hi",
		`{"choice": 1, "why": "x", "intent": "y"}`,
		`{"summary":"s","lately":"l","people":[],"done":[],"threads":[]}`,
	)
	s := sceneWith(Turn{UserID: id, Username: "Big M", Content: "hello", At: noon})
	s.UserID = id
	s.Roles = map[string]string{id: "a regular"}
	k, _ := m.Know(s)
	a, err := m.Consider(context.Background(), s, k)
	if err != nil {
		t.Fatal(err)
	}
	_ = m.Absorb(s, a)
	k, _ = m.Know(s)
	if _, _, err := m.Speak(context.Background(), s, k, a, ""); err != nil {
		t.Fatal(err)
	}
	_ = m.Said(s, a, "hi", "")
	k, _ = m.Know(Scene{GuildID: guildID, Now: noon}, id)
	_, _ = m.Initiate(context.Background(), Scene{GuildID: guildID, Now: noon}, k,
		[]Opening{{Trigger: TriggerReach, ChannelName: "chat", UserID: id, Username: "Big M"}})
	_, _ = m.Reflect(context.Background(), guildID, "Test", noon, noon.Add(15*time.Hour))

	if len(p.sent) != 4 {
		t.Fatalf("%d calls made", len(p.sent))
	}
	for _, msgs := range p.sent {
		for _, msg := range msgs {
			if strings.Contains(msg.Content, id) {
				t.Fatalf("an id reached the model:\n%s", msg.Content)
			}
		}
	}
}

// Weight is what makes a moment outlast its fortnight.
func TestAMomentThatHitHardOutlastsSmallTalk(t *testing.T) {
	m, _ := newMind(t)
	month := noon.Add(-30 * 24 * time.Hour)
	for _, mo := range []memory.Moment{
		{At: month, People: []memory.Ref{{ID: "123", Name: "Big M"}}, Text: "he said he could not stand me", Weight: 0.9},
		{At: month.Add(time.Minute), People: []memory.Ref{{ID: "123", Name: "Big M"}}, Text: "he said hi", Weight: 0.1},
	} {
		if err := m.Memory.AddMoment(guildID, mo); err != nil {
			t.Fatal(err)
		}
	}
	k, err := m.Know(sceneWith(him("hey", noon)))
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Recalled) != 1 || k.Recalled[0].Text != "he said he could not stand me" {
		t.Fatalf("recalled %+v", k.Recalled)
	}
}

func TestInitiateOnlyActsOnARealChoice(t *testing.T) {
	openings := []Opening{{Trigger: TriggerReach, ChannelName: "chat", UserID: "123", Username: "Big M"}}
	for reply, want := range map[string]int{
		`{"choice": 1, "why": "I want to know how the naming went", "intent": "ask about the name"}`: 0,
		`{"choice": 0, "why": "", "intent": ""}`:                                                     -1,
		`{"choice": 3, "why": "x", "intent": "y"}`:                                                   -1,
		`{"choice": 1, "why": "x", "intent": ""}`:                                                    -1,
	} {
		m, _ := newMind(t, reply)
		s := Scene{GuildID: guildID, Now: noon}
		k, _ := m.Know(s, "123")
		plan, err := m.Initiate(context.Background(), s, k, openings)
		if err != nil || plan.Choice != want {
			t.Errorf("%s: choice %d, %v; want %d", reply, plan.Choice, err, want)
		}
	}
}

// Seen in production: a quote inside a value, left unescaped, cost a whole
// appraisal.
func TestAnAppraisalWithUnescapedQuotesIsStillRead(t *testing.T) {
	a, ok := parseAppraisal("{\n  \"read\": \"He's testing her with a soft, \"haha\"-shielded compliment.\",\n  \"feel\": \"A little tired of it.\",\n  \"act\": \"reply\",\n  \"intent\": \"take it lightly\",\n  \"later_hours\": 12\n}")
	if !ok || a.Act != ActReply || a.Read != `He's testing her with a soft, "haha"-shielded compliment.` || a.LaterHours != 12 {
		t.Fatalf("read as %+v, %v", a, ok)
	}
}

// Seen in production: one impression noted six times in fifteen minutes.
func TestARewordedNoteIsNotKeptTwice(t *testing.T) {
	m, _ := newMind(t)
	s := sceneWith(him("hi", noon))
	for _, note := range []string{
		"He persists in trying to engage even when she's not reciprocating, but doesn't escalate when ignored.",
		"He's persistent in trying to engage, even when she's not reciprocating, but doesn't escalate when rebuffed.",
		"Works on a code-city visualiser.",
	} {
		if err := m.Absorb(s, Appraisal{Note: note}); err != nil {
			t.Fatal(err)
		}
	}
	p, _, _ := m.Memory.Person(guildID, "123")
	if len(p.Notes) != 2 {
		t.Fatalf("notes: %+v", p.Notes)
	}
}

func TestTheSameClosingLineIsARepeat(t *testing.T) {
	turns := []Turn{her("because 9 8 1. now, if youre done, i was in the middle of something", noon)}
	if _, ok := RepeatsHerself("Thats a reach, even for you. Now, if you'll excuse me, I was in the middle of something", turns); !ok {
		t.Error("the same sign-off twice was let through")
	}
}
