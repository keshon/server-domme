package mind

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// The scene from production: a question asked and answered at 12:43, a
// quiet afternoon, and at 16:20 the transcript is eight lines with none of
// it. The arc is what puts it back in front of her.
func TestArcCarriesTheConversationAcrossAPause(t *testing.T) {
	m, p := newMind(t, `{"read": "he is back", "act": "reply", "intent": "pick up where we were"}`)
	early := sceneWith(him("I'm still the one who is easy to confuse", noon))
	a := Appraisal{Arc: `I asked Big M what "easy to confuse" meant and he answered: nerves around someone important.`}
	if err := m.Absorb(early, a); err != nil {
		t.Fatal(err)
	}

	later := sceneWith(him("that silence in response...", noon.Add(3*time.Hour+30*time.Minute)))
	later.Now = noon.Add(3*time.Hour + 30*time.Minute)
	k, err := m.Know(later)
	if err != nil {
		t.Fatal(err)
	}
	if k.Arc == nil {
		t.Fatal("the arc was not carried across the pause")
	}
	if _, err := m.Consider(context.Background(), later, k); err != nil {
		t.Fatal(err)
	}
	prompt := p.sent[0][1].Content
	if !strings.Contains(prompt, `what "easy to confuse" meant and he answered`) {
		t.Errorf("the arc is not in what she is shown:\n%s", prompt)
	}
	if i, j := strings.Index(prompt, "How the conversation here has gone"), strings.Index(prompt, "The conversation, oldest first"); i < 0 || i > j {
		t.Errorf("the arc should come just before the transcript")
	}
}

// Rewritten, an arc keeps when it started and who was in it; left empty,
// the one there stands.
func TestArcKeepsItsStartAndPeople(t *testing.T) {
	m, _ := newMind(t)
	s := sceneWith(him("hi", noon))
	if err := m.Absorb(s, Appraisal{Arc: "Big M said hi."}); err != nil {
		t.Fatal(err)
	}

	s2 := sceneWith(him("hello", noon.Add(time.Hour)))
	s2.Now, s2.UserID, s2.Username = noon.Add(time.Hour), "456", "Duchess"
	if err := m.Absorb(s2, Appraisal{Arc: "Big M said hi, then Duchess joined."}); err != nil {
		t.Fatal(err)
	}
	s3 := s2
	s3.Now = noon.Add(2 * time.Hour)
	if err := m.Absorb(s3, Appraisal{}); err != nil {
		t.Fatal(err)
	}

	arc, ok, err := m.Memory.Arc(guildID, "c1")
	if err != nil || !ok {
		t.Fatalf("Arc = %v, %v", ok, err)
	}
	if arc.Text != "Big M said hi, then Duchess joined." || !arc.Started.Equal(noon) || !arc.Updated.Equal(noon.Add(time.Hour)) {
		t.Errorf("arc = %+v", arc)
	}
	if len(arc.People) != 2 || arc.People[0].ID != "123" || arc.People[1].ID != "456" {
		t.Errorf("people = %+v", arc.People)
	}
	if arc.Source.Kind != memory.Interpreted {
		t.Errorf("an arc is her reading, not %q", arc.Source.Kind)
	}
}

// A conversation over is not in front of her: a stale arc is left out, and
// the next write closes it into the day before starting a new one.
func TestStaleArcClosesIntoTheDay(t *testing.T) {
	m, _ := newMind(t)
	if err := m.Absorb(sceneWith(him("hi", noon)), Appraisal{Arc: "the old talk"}); err != nil {
		t.Fatal(err)
	}

	next := sceneWith(him("morning", noon.Add(arcFresh+time.Hour)))
	next.Now = noon.Add(arcFresh + time.Hour)
	k, err := m.Know(next)
	if err != nil {
		t.Fatal(err)
	}
	if k.Arc != nil {
		t.Error("a stale arc is still in front of her")
	}
	if err := m.Absorb(next, Appraisal{Arc: "a new talk"}); err != nil {
		t.Fatal(err)
	}
	arc, _, _ := m.Memory.Arc(guildID, "c1")
	if arc.Text != "a new talk" || !arc.Started.Equal(next.Now) {
		t.Errorf("new arc = %+v, want one starting now", arc)
	}
	day, err := m.Memory.Day(guildID, noon)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, mo := range day.Moments {
		found = found || strings.Contains(mo.Text, "the old talk")
	}
	if !found {
		t.Errorf("the old arc is not in its day: %+v", day.Moments)
	}
}

// Looking back on a day closes its conversations into it first; asked to
// reflect on today, she leaves today's conversation going.
func TestReflectClosesTheDaysArcs(t *testing.T) {
	m, _ := newMind(t)
	if err := m.Absorb(sceneWith(him("hi", noon)), Appraisal{Arc: "a talk"}); err != nil {
		t.Fatal(err)
	}

	_, _ = m.Reflect(context.Background(), guildID, "Test", noon, noon.Add(time.Hour), nil)
	if _, ok, _ := m.Memory.Arc(guildID, "c1"); !ok {
		t.Fatal("reflecting early on today closed today's conversation")
	}

	_, _ = m.Reflect(context.Background(), guildID, "Test", noon, noon.Add(18*time.Hour), nil)
	if _, ok, _ := m.Memory.Arc(guildID, "c1"); ok {
		t.Error("the day's arc was not closed when she looked back on it")
	}
}
