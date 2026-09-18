package mind

import (
	"strings"
	"testing"
	"time"
)

func TestParseNotesReadsWhatRelaysActuallySend(t *testing.T) {
	at := time.Now()
	reply := `Here are my notes:
- **FACT Big M**: Job = night shift nurse
FACT big m: the pet = a cat called Bo
IMPRESSION Big M: talks big, backs it up, fun to needle.
FACT cass: city = Lisbon
something unrelated
FACT nobody without a colon
IMPRESSION : empty name`

	got := ParseNotes(reply, at)
	if len(got) != 2 {
		t.Fatalf("got %d people, want 2: %+v", len(got), got)
	}

	m := got[0]
	if m.Name != "Big M" || len(m.Facts) != 2 || m.Impression == "" {
		t.Fatalf("Big M parsed as %+v", m)
	}
	if m.Facts[0].Key != "job" || m.Facts[0].Value != "night shift nurse" {
		t.Errorf("first fact %+v", m.Facts[0])
	}
	if m.Facts[1].Key != "pet" {
		t.Errorf("key not normalised: %q", m.Facts[1].Key)
	}
	if got[1].Name != "cass" || got[1].Facts[0].Value != "Lisbon" {
		t.Errorf("cass parsed as %+v", got[1])
	}
}

func TestParseNotesAcceptsNone(t *testing.T) {
	if got := ParseNotes("NONE", time.Now()); len(got) != 0 {
		t.Errorf("NONE produced %+v", got)
	}
}

// People change jobs. The first answer kept forever is how she ends up
// confidently wrong about someone — cognitum's semantic memory did exactly
// that.
func TestMergeFactsLetsTheNewestValueWin(t *testing.T) {
	old := time.Now().Add(-30 * 24 * time.Hour)
	now := time.Now()
	known := []Fact{{Key: "job", Value: "barista", At: old}, {Key: "pet", Value: "a cat", At: old}}

	got := MergeFacts(known, []Fact{{Key: "job", Value: "nurse", At: now}})

	if len(got) != 2 || got[0].Key != "job" || got[0].Value != "nurse" {
		t.Errorf("MergeFacts = %+v, want the new job first and the pet kept", got)
	}
}

func TestMergeFactsKeepsTheNewestWhenFull(t *testing.T) {
	base := time.Now().Add(-time.Hour)
	var known []Fact
	for i := 0; i < MaxFacts; i++ {
		known = append(known, Fact{Key: string(rune('a' + i)), Value: "x", At: base.Add(time.Duration(i) * time.Minute)})
	}
	got := MergeFacts(known, []Fact{{Key: "newest", Value: "y", At: time.Now()}})

	if len(got) != MaxFacts || got[0].Key != "newest" {
		t.Fatalf("MergeFacts kept %d, first %q", len(got), got[0].Key)
	}
	for _, f := range got {
		if f.Key == "a" {
			t.Error("kept the oldest fact over the newest")
		}
	}
}

func TestWarmthFadesOverWeeksNotHours(t *testing.T) {
	now := time.Now()
	if got := WarmthNow(0.8, now.Add(-24*time.Hour), now); got < 0.75 {
		t.Errorf("a day later warmth is %.2f; liking someone does not pass overnight", got)
	}
	if got := WarmthNow(0.8, now.Add(-WarmthHalflife), now); got < 0.39 || got > 0.41 {
		t.Errorf("one halflife later warmth is %.2f, want half", got)
	}
}

func TestWarmthStepAttributesLikeIrritationDoes(t *testing.T) {
	if WarmthStep(ToneWarm, true) <= WarmthStep(ToneWarm, false) {
		t.Error("a warm group conversation counted as much as a warm one-to-one")
	}
	if WarmthStep(ToneHostile, false) != 0 {
		t.Error("a hostile group conversation cooled her towards everyone in it")
	}
	if WarmthStep(ToneHostile, true) >= 0 {
		t.Error("a hostile one-to-one did not cool her")
	}
}

func TestPeopleBlockCarriesWhatSheKnows(t *testing.T) {
	g := Grounding{Present: []Acquaintance{{
		UserID: "1", Username: "Big M", Messages: 40,
		Facts:      []Fact{{Key: "job", Value: "night shift nurse"}},
		Impression: "talks big, backs it up",
	}}}
	got := g.Render()
	for _, want := range []string{"job: night shift nurse", "your take: talks big, backs it up"} {
		if !strings.Contains(got, want) {
			t.Errorf("people block is missing %q:\n%s", want, got)
		}
	}
}

// Being told to be short with someone and to go easy on them in one prompt is
// a contradiction the model resolves at random.
func TestFondnessIsNeverDirectedAtSomeoneShesShortWith(t *testing.T) {
	g := Grounding{Present: []Acquaintance{{
		UserID: "1", Username: "Big M", Irritation: 0.9, Warmth: 0.9,
	}}}
	got := buildSystem(nil, g, DefaultBudget())
	if strings.Contains(got, "fond of Big M") {
		t.Errorf("told to be fond of someone she is also told to be short with:\n%s", got)
	}
}
