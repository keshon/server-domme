package mind

import (
	"strings"
	"testing"
	"time"
)

func at(hour int) time.Time {
	return time.Date(2026, 9, 12, hour, 0, 0, 0, time.UTC)
}

func TestCircadianIsLowestInTheSmallHours(t *testing.T) {
	night := circadian(at(4), time.UTC)
	morning := circadian(at(10), time.UTC)
	evening := circadian(at(20), time.UTC)

	if !(night < morning && morning < evening) {
		t.Errorf("energy should climb 04:00 -> 10:00 -> 20:00, got %.2f %.2f %.2f",
			night, morning, evening)
	}
	if night > 0.25 {
		t.Errorf("4am energy = %.2f, want it low enough to show in the phrase", night)
	}
	if evening < 0.8 {
		t.Errorf("8pm energy = %.2f, want it high", evening)
	}

	// The bug this replaced: a symmetric curve with its trough at 04:00 made
	// 08:00 just as dark.
	if breakfast := circadian(at(8), time.UTC); breakfast < 0.4 {
		t.Errorf("8am energy = %.2f, want someone who is awake", breakfast)
	}
}

// The timezone is the community's, not the rack's: 4am UTC is the evening in
// Auckland, and a bot yawning through someone's prime time is worse than one
// with no clock at all.
func TestCircadianFollowsTheConfiguredZone(t *testing.T) {
	auckland := time.FixedZone("NZST", 12*60*60)

	utcNight := circadian(at(4), time.UTC)
	sameMomentInAuckland := circadian(at(4), auckland)

	if sameMomentInAuckland <= utcNight {
		t.Errorf("same instant should read as evening in +12: utc=%.2f akl=%.2f",
			utcNight, sameMomentInAuckland)
	}
}

func TestSolitudeSaturates(t *testing.T) {
	now := at(12)

	if got := solitude(now, now); got != 0 {
		t.Errorf("just spoke: solitude = %.2f, want 0", got)
	}
	if got := solitude(now, now.Add(-solitudeFull*2)); got != 1 {
		t.Errorf("long gone: solitude = %.2f, want 1", got)
	}
	if got := solitude(now, time.Time{}); got != 1 {
		t.Errorf("never spoke: solitude = %.2f, want 1 — silence is silence", got)
	}

	half := solitude(now, now.Add(-solitudeFull/2))
	if half < 0.4 || half > 0.6 {
		t.Errorf("half the window: solitude = %.2f, want about 0.5", half)
	}
}

func TestInterestNeedsSomethingToBeInterestedIn(t *testing.T) {
	if got := interest(0, 0); got != 0 {
		t.Errorf("empty channel: interest = %.2f, want 0", got)
	}
	busy := interest(busyChannel, busyChannel)
	quiet := interest(1, 0)
	if busy <= quiet {
		t.Errorf("a busy room aimed at her should beat one idle line: %.2f vs %.2f", busy, quiet)
	}
}

func TestDeriveDrivesUsesEveryInput(t *testing.T) {
	now := at(3)
	d := DeriveDrives(MoodInput{
		Now:            now,
		LastSpokeAt:    now.Add(-solitudeFull),
		RecentTurns:    busyChannel,
		AddressedTurns: busyChannel,
	})

	if d.Energy > 0.25 {
		t.Errorf("3am Energy = %.2f, want low", d.Energy)
	}
	if d.Social < 0.9 {
		t.Errorf("a full window of silence: Social = %.2f, want near 1", d.Social)
	}
	if d.Arousal < 0.9 {
		t.Errorf("busy and aimed at her: Interest = %.2f, want near 1", d.Arousal)
	}
}

// Someone tired still answers when spoken to. Making a direct approach
// conditional on mood is how "she ignored my direct question" comes back with
// a better excuse.
func TestNudgeLeavesDirectApproachesAlone(t *testing.T) {
	a := DefaultAttention()
	now := at(3) // the worst possible mood: small hours
	exhausted := DeriveDrives(MoodInput{Now: now, LastSpokeAt: now})

	for _, trigger := range []Trigger{TriggerMention, TriggerReply} {
		s := Situation{Trigger: trigger, Now: now, Drives: exhausted, LastSpokeAt: now.Add(-time.Minute)}
		if got := Decide(a, s, 0.9999); got != OutcomeSpeak {
			t.Errorf("%s at 3am: Decide = %q on the worst roll, want %q", trigger, got, OutcomeSpeak)
		}
	}
}

func TestNudgeMovesTheIndirectOnes(t *testing.T) {
	a := DefaultAttention()
	now := at(20)

	lonely := Drives{Social: 1, Energy: 0.9, Arousal: 1}
	flat := Drives{Social: 0, Energy: 0.15, Arousal: 0}

	if lonely.Nudge() <= flat.Nudge() {
		t.Fatalf("lonely and awake should want to talk more than tired and ignored: %.2f vs %.2f",
			lonely.Nudge(), flat.Nudge())
	}

	// A roll that falls between the two shifted thresholds is answered in one
	// mood and not the other, which is the whole point of the mechanism.
	roll := a.NamedChance + (lonely.Nudge()+flat.Nudge())/2
	loud := Decide(a, Situation{Trigger: TriggerNamed, Now: now, Drives: lonely}, roll)
	quiet := Decide(a, Situation{Trigger: TriggerNamed, Now: now, Drives: flat}, roll)

	if loud != OutcomeSpeak || quiet != OutcomeIgnore {
		t.Errorf("same roll, different moods: lonely=%q flat=%q, want speak/ignore", loud, quiet)
	}
}

func TestNudgeIsBounded(t *testing.T) {
	best := Drives{Social: 1, Energy: 1, Arousal: 1}
	worst := Drives{Social: 0, Energy: 0, Arousal: 0}

	if got := best.Nudge(); got > 0.25 {
		t.Errorf("Nudge = %.2f, want it capped: mood adjusts the odds, it does not set them", got)
	}
	if got := worst.Nudge(); got < -0.2 {
		t.Errorf("Nudge = %.2f, want it floored", got)
	}
}

// "Not computed" must not read as "exhausted and friendless".
func TestNudgeTreatsTheZeroValueAsNeutral(t *testing.T) {
	if got := (Drives{}).Nudge(); got != 0 {
		t.Errorf("zero Drives nudged by %.2f, want 0 — an unset mood is not a bad one", got)
	}
}

// Instructions, not descriptions. Stated as a mood in the grounding this
// changed nothing measurable: at 3am the character wrote the longest and
// liveliest reply of the set.
func TestDirectivesTellHerHowToWriteNotHowSheFeels(t *testing.T) {
	joined := Grounding{Drives: Drives{Social: 0.9, Energy: 0.12, Arousal: 0.2}}.State()
	if joined == "" {
		t.Fatal("a wrung-out character was given no instruction at all")
	}
	if !strings.Contains(joined, "few words") && !strings.Contains(joined, "shorter") {
		t.Errorf("nothing here says how to write:\n%q", joined)
	}
}

func TestDirectivesSayNothingAboutAnUnremarkableMood(t *testing.T) {
	if got := (Grounding{Drives: Drives{Social: 0.2, Energy: 0.6, Arousal: 0.3}}).State(); got != "" {
		t.Errorf("an ordinary mood produced instructions: %q", got)
	}
	if got := (Grounding{}).State(); got != "" {
		t.Errorf("an unset mood produced instructions: %q", got)
	}
}

// A list of qualifications on every reply is how a strong instruction becomes
// a weak one, which this prompt has demonstrated three times.
func TestDirectivesStayShort(t *testing.T) {
	everything := Grounding{
		Drives: Drives{Social: 1, Energy: 0, Arousal: 1, Mood: -1},
		Present: []Acquaintance{
			{Username: "Big M", Tension: 0.9},
			{Username: "cass", Closeness: 0.9},
		},
	}
	if got := everything.stateSentences(); len(got) > 4 {
		t.Errorf("gave %d instructions at once: %q", len(got), got)
	}
}
