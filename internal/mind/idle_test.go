package mind

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Step 7 of docs/persona-v3.md: the idle mind, walks, impulses, life.

func TestTheIdleMindKeepsWhatIsOnHerMindAndWhatCaughtHer(t *testing.T) {
	m, p := newMind(t, `{"on_mind":"that dragon sketch, the wings","caught":"a dragon sketch with wings on backwards","caught_weight":0.9,"impulse":{"to":"Rook","about":"tell him the wings are backwards"}}`)
	in := Idle{
		GuildID: guildID, Now: noon,
		Walk:   &Walk{Channel: "art", Lines: []Turn{{UserID: "5", Username: "Rook", Content: "new dragon!", At: noon}}},
		People: []Candidate{{ID: "5", Name: "Rook", Here: true}},
	}
	res, err := m.IdleThink(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	self, _ := m.Memory.Self(guildID)
	if self.OnMind != "that dragon sketch, the wings" {
		t.Errorf("on her mind: %q", self.OnMind)
	}
	day, _ := m.Memory.Day(guildID, noon)
	if len(day.Moments) != 1 || !day.Moments[0].Walk || day.Moments[0].Weight > maxWalkWeight {
		t.Errorf("walk moment %+v", day.Moments)
	}
	if res.Impulse == nil || res.Impulse.Person == nil || res.Impulse.Person.ID != "5" {
		t.Errorf("impulse %+v", res.Impulse)
	}
	if !strings.Contains(p.sent[0][1].Content, "new dragon!") {
		t.Error("the walk's lines were not shown")
	}
}

// An impulse at someone who was not offered is a name the model made up;
// at a room, only where she may speak up.
func TestImpulsesOnlyAtWhatWasOffered(t *testing.T) {
	for reply, rooms := range map[string]bool{
		`{"on_mind":"x","impulse":{"to":"Nobody Real","about":"hi"}}`: true,
		`{"on_mind":"x","impulse":{"to":"a room","about":"hi"}}`:      false,
	} {
		m, _ := newMind(t, reply)
		res, err := m.IdleThink(context.Background(), Idle{GuildID: guildID, Now: noon, Rooms: rooms})
		if err != nil || res.Impulse != nil {
			t.Errorf("%s: impulse %+v, %v", reply, res.Impulse, err)
		}
	}
}

// Her life rests on what happened: a new item cites moments; one that
// cites nothing is refused; one kept without a moment behind it stays
// exactly as it was.
func TestLifeRestsOnWhatHappened(t *testing.T) {
	m, _ := newMind(t, `{
		"life":[
			{"text":"everyone in #art is drawing dragons","moments":[1]},
			{"text":"made up out of nothing","moments":[]},
			{"text":"rewritten without cause","keeps":1}
		],
		"wants":[{"text":"see Rook finish the dragon","why":"he never finishes anything"},{"keeps":1,"came_up":true}]
	}`)
	day := noon.Add(-24 * time.Hour)
	if err := m.Memory.UpdateSelf(guildID, func(me *memory.Self) {
		me.Life = []memory.LifeItem{{Text: "the server is quiet", Since: day, Advanced: day, Sources: []string{"x"}}}
		me.Wants = []memory.Want{{Text: "be left alone about the rules", Since: day, Touched: day.Add(-48 * time.Hour)}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Memory.AddMoment(guildID, memory.Moment{At: day.Add(time.Hour), Channel: "art", Text: "passed through #art: dragons", Walk: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReflectLife(context.Background(), guildID, day, noon); err != nil {
		t.Fatal(err)
	}
	self, _ := m.Memory.Self(guildID)
	if len(self.Life) != 2 {
		t.Fatalf("life %+v", self.Life)
	}
	if self.Life[0].Text != "everyone in #art is drawing dragons" || len(self.Life[0].Sources) != 1 {
		t.Errorf("new item %+v", self.Life[0])
	}
	if self.Life[1].Text != "the server is quiet" {
		t.Errorf("a kept item changed without a moment: %+v", self.Life[1])
	}
	if len(self.Wants) != 2 || !self.Wants[1].Touched.Equal(startOf(day)) {
		t.Errorf("wants %+v", self.Wants)
	}
	if self.LifeThrough.IsZero() {
		t.Error("the day was not marked")
	}
}

func TestInnerLifeIsInFrontOfHer(t *testing.T) {
	self := memory.Self{
		OnMind: "the dragon", OnMindAt: noon.Add(-time.Hour),
		Life:  []memory.LifeItem{{Text: "dragons everywhere", Advanced: noon}},
		Wants: []memory.Want{{Text: "see it finished", Why: "curious", Touched: noon}},
	}
	got := renderWorld(Scene{Now: noon}, Known{Self: self})
	for _, want := range []string{"On her mind: the dragon", "dragons everywhere", "see it finished — curious"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	self.OnMindAt = noon.Add(-10 * time.Hour)
	self.Life[0].Advanced = noon.Add(-30 * 24 * time.Hour)
	got = renderWorld(Scene{Now: noon}, Known{Self: self})
	if strings.Contains(got, "the dragon") || strings.Contains(got, "dragons everywhere") {
		t.Errorf("stale inner life shown:\n%s", got)
	}
}
