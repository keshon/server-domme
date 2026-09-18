package chat

import (
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

func interviewTomorrow(t *testing.T, svc *Service, said time.Time) {
	t.Helper()
	turns := []mind.Turn{{UserID: "u1", Username: "Big M", Content: "got a job interview tomorrow at the clinic", At: said}}
	svc.noteConcerns(testGuild, "u1", []mind.Plan{
		{What: "clinic interview", When: "tomorrow"},
		{What: "wedding in Porto", When: "tomorrow"}, // never said: must not be kept
	}, turns, said)
}

func TestPlansInTheirOwnWordsBecomeConcerns(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	interviewTomorrow(t, svc, time.Now())

	p := store.GetMindPerson(testGuild, "u1")
	if p == nil || len(p.Concerns) != 1 || p.Concerns[0].What != "clinic interview" {
		t.Fatalf("concerns %+v", p)
	}
}

// On her mind with the probability of its salience; raised, it is done;
// let pass, it weakens.
func TestAConcernIsOfferedThenRaisedOrLetPass(t *testing.T) {
	store := testStore(t)
	now := time.Now()
	sure := newTestService(t, store, 0)
	interviewTomorrow(t, sure, now.Add(-30*time.Hour))
	// Close to him, so it matters.
	if err := store.UpdateMindPerson(testGuild, "u1", now, func(p *storage.MindPerson) {
		p.Closeness, p.ClosenessAt = 0.8, now
	}); err != nil {
		t.Fatal(err)
	}

	what, phrase := sure.concernOnMind(testGuild, "u1", "Big M", 0, now)
	if what == "" || phrase == "" {
		t.Fatal("a salient concern was not on her mind on the surest roll")
	}
	never := newTestService(t, store, 0.9999)
	if w, _ := never.concernOnMind(testGuild, "u1", "Big M", 0, now); w != "" {
		t.Error("on her mind on a roll above its salience")
	}

	sure.afterConcern(testGuild, "u1", what, "sounds exhausting, go to bed", now)
	if p := store.GetMindPerson(testGuild, "u1"); p.Concerns[0].Passed != 1 {
		t.Errorf("letting it pass was not counted: %+v", p.Concerns)
	}
	sure.afterConcern(testGuild, "u1", what, "how did the clinic interview go", now)
	if p := store.GetMindPerson(testGuild, "u1"); len(p.Concerns) != 0 {
		t.Errorf("raising it did not close it: %+v", p.Concerns)
	}
}

// The world can answer before she asks: once it is due, their own mention of
// it closes it. Before then, talking about it is just more of the plan.
func TestTheirOwnNewsClosesAConcern(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	said := time.Now().Add(-30 * time.Hour)
	interviewTomorrow(t, svc, said)

	p := store.GetMindPerson(testGuild, "u1")
	svc.resolveConcerns(testGuild, "u1", "so nervous about the clinic interview", p, said.Add(time.Hour))
	if p = store.GetMindPerson(testGuild, "u1"); len(p.Concerns) != 1 {
		t.Fatal("talking about it beforehand closed it")
	}
	svc.resolveConcerns(testGuild, "u1", "clinic interview went great, got the job", p, time.Now())
	if p = store.GetMindPerson(testGuild, "u1"); len(p.Concerns) != 0 {
		t.Errorf("their news did not close it: %+v", p.Concerns)
	}
}
