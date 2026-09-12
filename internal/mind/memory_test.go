package mind

import (
	"strings"
	"testing"
	"time"
)

func mem(gist, detail string, ageDays float64, weight float64, people ...string) Memory {
	return Memory{
		At:     time.Now().Add(-time.Duration(ageDays * float64(24*time.Hour))),
		Gist:   gist,
		Detail: detail,
		Weight: weight,
		People: people,
	}
}

func TestBrightnessFadesWithAge(t *testing.T) {
	now := time.Now()

	today := mem("an argument", "about the rules", 0, 0).Brightness(now, nil, nil)
	lastWeek := mem("an argument", "about the rules", 7, 0).Brightness(now, nil, nil)
	lastMonth := mem("an argument", "about the rules", 30, 0).Brightness(now, nil, nil)

	if !(today > lastWeek && lastWeek > lastMonth) {
		t.Errorf("brightness should fall with age: %.3f %.3f %.3f", today, lastWeek, lastMonth)
	}
	if lastMonth > brightnessFloor {
		t.Errorf("a month-old unremarkable memory is still lit at %.3f", lastMonth)
	}
}

// cognitum's §5.6: an emotional peak decays more slowly. It is what makes a
// charged day outlast the small talk around it.
func TestWeightOutlastsOrdinaryChatter(t *testing.T) {
	now := time.Now()

	ordinary := mem("someone posted a link", "", 10, 0).Brightness(now, nil, nil)
	charged := mem("the row that split the server", "", 10, 1).Brightness(now, nil, nil)

	if charged <= ordinary {
		t.Errorf("a weighted memory should outlast an ordinary one: %.3f vs %.3f", charged, ordinary)
	}
	if ordinary > brightnessFloor {
		t.Errorf("ten-day-old small talk should be gone, got %.3f", ordinary)
	}
	if charged < brightnessFloor {
		t.Errorf("a charged memory should survive ten days, got %.3f", charged)
	}
}

// Weight slows the fade rather than raising the level: an old charged memory
// must not read as more present than something that just happened.
func TestWeightDoesNotOutshineTheRecent(t *testing.T) {
	now := time.Now()

	justNow := mem("someone said hello", "", 0, 0).Brightness(now, nil, nil)
	oldAndCharged := mem("the row that split the server", "", 20, 1).Brightness(now, nil, nil)

	if oldAndCharged >= justNow {
		t.Errorf("a twenty-day-old memory outshone this minute: %.3f vs %.3f", oldAndCharged, justNow)
	}
}

// cognitum P6: old thoughts return when a trigger matches their topic.
func TestTopicBringsAnOldMemoryBack(t *testing.T) {
	now := time.Now()
	old := mem("argument about purge rules", "cass and newbie went at it", 12, 0)

	cold := old.Brightness(now, Keywords("anyone seen the new emotes"), nil)
	warm := old.Brightness(now, Keywords("are we changing the purge rules again"), nil)

	if warm <= cold {
		t.Errorf("mentioning the subject should raise it: %.3f vs %.3f", warm, cold)
	}
	if cold > brightnessFloor {
		t.Errorf("unprompted, a twelve-day-old memory should be out: %.3f", cold)
	}
	if warm < brightnessFloor {
		t.Errorf("prompted, it should come back: %.3f", warm)
	}
}

// The other half of what makes recall feel personal: who was there.
func TestPresenceBringsAMemoryBack(t *testing.T) {
	now := time.Now()
	old := mem("a long argument", "it went on for hours", 8, 0, "u1", "u2")

	alone := old.Brightness(now, nil, []string{"u9"})
	withCass := old.Brightness(now, nil, []string{"u1"})

	if withCass <= alone {
		t.Errorf("someone who was there should raise it: %.3f vs %.3f", withCass, alone)
	}
}

func TestRecallDropsWhatIsBelowTheFloorAndSortsByBrightness(t *testing.T) {
	now := time.Now()
	memories := []Memory{
		mem("ancient trivia", "", 60, 0),
		mem("yesterday's disagreement", "about pins", 1, 0),
		mem("last week's quiet spell", "", 6, 0),
		{At: now, Gist: "  "}, // no gist: nothing to remember
	}

	got := Recall(memories, now, nil, nil, 10)

	for _, m := range got {
		if strings.TrimSpace(m.Gist) == "" {
			t.Error("recalled a memory with no gist")
		}
		if m.Gist == "ancient trivia" {
			t.Error("recalled something two months old and unremarkable")
		}
	}
	if len(got) < 2 {
		t.Fatalf("recalled %d memories, want the recent ones", len(got))
	}
	if got[0].Gist != "yesterday's disagreement" {
		t.Errorf("brightest first: got %q", got[0].Gist)
	}
}

func TestRecallRespectsTheCap(t *testing.T) {
	now := time.Now()
	var many []Memory
	for i := 0; i < 20; i++ {
		many = append(many, mem("thing", "detail", 0, 0))
	}

	if got := Recall(many, now, nil, nil, 3); len(got) != 3 {
		t.Errorf("recalled %d, want 3", len(got))
	}
}

// The gradient: full while bright, then clipped, then a bare hint.
func TestRenderFadesFromDetailToHint(t *testing.T) {
	now := time.Now()
	detail := "cass and newbie went back and forth over whether pinned messages should " +
		"survive a purge, nobody conceded, and it ran until well past midnight"

	fresh := mem("argument about purge rules", detail, 0, 0).recollection(now, nil, nil)
	middling := mem("argument about purge rules", detail, 3, 0).recollection(now, nil, nil)
	faded := mem("argument about purge rules", detail, 12, 0).recollection(now, nil, nil)

	if !strings.Contains(fresh, "went back and forth") {
		t.Errorf("a fresh memory should carry its detail: %q", fresh)
	}
	if len(middling) >= len(fresh) {
		t.Errorf("a middling memory should be shorter than a fresh one:\n  %q\n  %q", middling, fresh)
	}
	if strings.Contains(faded, "went back and forth") {
		t.Errorf("an old memory should be a hint, not an account: %q", faded)
	}
	if !strings.Contains(faded, "argument about purge rules") {
		t.Errorf("the gist is what survives: %q", faded)
	}
	// Someone who barely remembers a thing does not know exactly when.
	if strings.Contains(faded, "ago") {
		t.Errorf("a faded memory should not be timestamped: %q", faded)
	}
}

func TestKeywordsDropsTheNoise(t *testing.T) {
	got := Keywords("What was the argument about the RULES?")

	found := make(map[string]bool, len(got))
	for _, w := range got {
		found[w] = true
	}
	if !found["argument"] || !found["rules"] {
		t.Errorf("lost the subject: %q", got)
	}
	for _, noise := range []string{"what", "was", "the", "about"} {
		if found[noise] {
			t.Errorf("kept the stopword %q: %q", noise, got)
		}
	}
}

func TestRenderIsEmptyWithNothingToRemember(t *testing.T) {
	if got := Render(nil, time.Now(), nil, nil); got != "" {
		t.Errorf("rendered %q for no memories", got)
	}
}
