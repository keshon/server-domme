package mind

import (
	"math"
	"testing"
	"time"
)

func TestMoodSwingFadesBackToBaseline(t *testing.T) {
	now := time.Now()
	m := MoodSwing{}.Add(-0.4, now.Add(-moodHalflife))
	if got := m.Now(now); math.Abs(got+0.2) > 0.01 {
		t.Errorf("one half-life after -0.4: %.2f, want -0.20", got)
	}
	if got := m.Now(now.Add(24 * time.Hour)); got != 0 {
		t.Errorf("a day later: %.2f, want gone", got)
	}
	for range 20 {
		m = m.Add(0.3, now)
	}
	if m.Now(now) > 1 {
		t.Errorf("mood ran past 1: %.2f", m.Now(now))
	}
}

func TestDayToneHoldsAllDayAndChangesTomorrow(t *testing.T) {
	morning := time.Date(2026, 9, 18, 8, 0, 0, 0, time.UTC)
	if DayTone("g1", morning, nil) != DayTone("g1", morning.Add(14*time.Hour), nil) {
		t.Error("the day's tone changed within the day")
	}
	same := 0
	for d := range 30 {
		day := morning.Add(time.Duration(d) * 24 * time.Hour)
		if DayTone("g1", day, nil) == DayTone("g1", day.Add(24*time.Hour), nil) {
			same++
		}
		if DayTone("g1", day, nil) == DayTone("g2", day, nil) {
			same++
		}
	}
	if same > 0 {
		t.Errorf("%d days identical to the next day or to another server", same)
	}
}

// Noticeable but rare: flat most days, a clearly good or bad one now and then,
// and no lean either way.
func TestDayToneIsMostlyFlat(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	const days = 3000
	var strong, flat int
	var sum float64
	for d := range days {
		tone := DayTone("g1", start.Add(time.Duration(d)*24*time.Hour), nil)
		sum += tone
		switch {
		case math.Abs(tone) > 0.6:
			strong++
		case math.Abs(tone) < 0.2:
			flat++
		}
	}
	if share := float64(strong) / days; share < 0.1 || share > 0.25 {
		t.Errorf("%.0f%% of days strongly good or bad, want about one in six", share*100)
	}
	if share := float64(flat) / days; share < 0.45 {
		t.Errorf("only %.0f%% of days flat", share*100)
	}
	if mean := sum / days; math.Abs(mean) > 0.05 {
		t.Errorf("days lean %+.2f on average", mean)
	}
}

func TestRepetitionSeesARoomGoingStale(t *testing.T) {
	stale := []Turn{
		{UserID: "a", Content: "the chicken crossed the road again tonight"},
		{UserID: "b", Content: "chicken road again, classic chicken"},
		{UserID: "a", Content: "that chicken and that road, tonight again"},
		{UserID: "b", Content: "road chicken tonight classic"},
	}
	varied := []Turn{
		{UserID: "a", Content: "the chicken crossed the road again tonight"},
		{UserID: "b", Content: "anyone watching the football later maybe"},
		{UserID: "a", Content: "cooking pasta, burnt garlic, disaster kitchen"},
		{UserID: "b", Content: "work tomorrow early meeting boss angry"},
	}
	if Repetition(stale) <= Repetition(varied) {
		t.Errorf("stale %.2f, varied %.2f", Repetition(stale), Repetition(varied))
	}
	if Repetition(stale[:1]) != 0 {
		t.Error("one line judged as repetitive")
	}

	own := append([]Turn{}, varied...)
	for range 4 {
		own = append(own, Turn{FromBot: true, Content: "chicken road tonight again classic"})
	}
	if Repetition(own) != Repetition(varied) {
		t.Error("her own lines counted towards the room going stale")
	}
}

func TestDeriveDrivesHabituatesAndFeels(t *testing.T) {
	now := at(15)
	base := MoodInput{Now: now, LastSpokeAt: now, RecentTurns: busyChannel, AddressedTurns: busyChannel / 2}
	fresh := DeriveDrives(base)

	stale := base
	stale.Repetition = 0.8
	if DeriveDrives(stale).Arousal >= fresh.Arousal {
		t.Error("the same subject again held her as much as a new one")
	}

	sour := base
	sour.Swing = -0.5
	if DeriveDrives(sour).Mood >= fresh.Mood {
		t.Error("a bad swing did not lower her mood")
	}
	warm := base
	warm.Baseline = 0.3
	if DeriveDrives(warm).Mood <= fresh.Mood {
		t.Error("a warm temperament did not lift her baseline")
	}
}

func TestABadMoodIsFeltInTheOddsAndTheWords(t *testing.T) {
	good := Drives{Energy: 0.8, Mood: 0.6}
	bad := Drives{Energy: 0.8, Mood: -0.6}
	if bad.Nudge() >= good.Nudge() {
		t.Errorf("bad mood nudge %.2f, good %.2f", bad.Nudge(), good.Nudge())
	}
	if (Grounding{Drives: bad}).State() == "" || (Grounding{Drives: good}).State() == "" {
		t.Error("a pronounced mood was not told to her")
	}
	if (Grounding{Drives: Drives{Energy: 0.8, Mood: 0.1}}).State() != "" {
		t.Error("an ordinary mood was told to her")
	}
	if MoodWords(bad) == MoodWords(good) {
		t.Error("the panel describes a good and a bad mood the same way")
	}
}

// Someone getting on her nerves leaves her short with everyone for a while —
// less than with them — and something going well lifts her.
func TestEventsSpillOverIntoHerMood(t *testing.T) {
	for e, s := range appraisals {
		if s.Tension > 0 && s.Mood > 0 {
			t.Errorf("%q annoys her and cheers her up", e)
		}
		// Irritation spills over, weaker than towards whoever caused it.
		if s.Tension > 0 && -s.Mood >= s.Tension {
			t.Errorf("%q sours her mood as much as it annoys her with them", e)
		}
	}
	if Appraise(EventPestered).Mood >= 0 || Appraise(EventLaughed).Mood <= 0 {
		t.Error("pestering and laughter do not reach her mood")
	}
	if ConversationMood(ToneHostile) >= 0 || ConversationMood(ToneWarm) <= 0 || ConversationMood(ToneOrdinary) != 0 {
		t.Error("conversation tone does not reach her mood")
	}
}

func TestMoodBaselineFollowsWarmth(t *testing.T) {
	warm, reserved := DefaultSpeechStyle(), DefaultSpeechStyle()
	warm.Warmth, reserved.Warmth = 0.9, 0.2
	if warm.MoodBaseline() <= 0 || reserved.MoodBaseline() >= 0 {
		t.Errorf("warm %.2f, reserved %.2f", warm.MoodBaseline(), reserved.MoodBaseline())
	}
	if DefaultSpeechStyle().MoodBaseline() != 0 || (SpeechStyle{}).MoodBaseline() != 0 {
		t.Error("an unremarkable temperament moved her baseline")
	}
}

func TestClosenessMakesHerMoreAvailable(t *testing.T) {
	a := DefaultAttention()
	s := Situation{Trigger: TriggerNamed, Now: time.Now()}
	_, cold := DecideWhy(a, s, 1)
	s.Closeness = 1
	_, near := DecideWhy(a, s, 1)
	if near.Chance <= cold.Chance {
		t.Errorf("close %.2f, a stranger %.2f", near.Chance, cold.Chance)
	}
	if near.Chance-cold.Chance > 0.1 {
		t.Errorf("closeness moved the odds by %.2f, more than a nudge", near.Chance-cold.Chance)
	}
}

// Stage three's promise: mood, the day and closeness make her answer rate
// vary with her state without moving it on average. Over a year of
// afternoons, with the closeness a typical room has, each trigger's mean odds
// stay within a few points of what they were with none of it, and the spread
// is real.
func TestAnswerRatesStayNearTodaysOnAverage(t *testing.T) {
	a := DefaultAttention()
	start := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	for _, trigger := range []Trigger{TriggerNamed, TriggerAbout, TriggerFollowUp} {
		in := MoodInput{Now: start, LastSpokeAt: start.Add(-time.Hour), RecentTurns: 6, AddressedTurns: 2}
		before := DeriveDrives(in)
		before.Energy, before.Mood = circadian(start, nil), 0
		_, old := DecideWhy(a, Situation{Trigger: trigger, Now: start, Drives: before}, 1)

		var sum, lo, hi float64
		lo = 1
		const days = 365
		for d := range days {
			in.Now = start.Add(time.Duration(d) * 24 * time.Hour)
			in.LastSpokeAt = in.Now.Add(-time.Hour)
			in.Seed = "g1"
			closeness := 0.3 * float64(d%10) / 10
			_, now := DecideWhy(a, Situation{Trigger: trigger, Now: in.Now, Drives: DeriveDrives(in), Closeness: closeness}, 1)
			sum += now.Chance
			lo, hi = math.Min(lo, now.Chance), math.Max(hi, now.Chance)
		}
		mean := sum / days
		t.Logf("%s: was %.2f, now %.2f on average (%.2f–%.2f)", trigger, old.Chance, mean, lo, hi)
		if math.Abs(mean-old.Chance) > 0.04 {
			t.Errorf("%s: average odds moved from %.2f to %.2f", trigger, old.Chance, mean)
		}
		if hi-lo < 0.05 {
			t.Errorf("%s: odds barely vary with her state (%.2f–%.2f)", trigger, lo, hi)
		}
	}
}
