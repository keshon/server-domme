package mind

import (
	"strings"
	"testing"
	"time"
)

func TestLongingGrowsFasterForPeopleSheLikes(t *testing.T) {
	now := time.Now()
	day := now.Add(-24 * time.Hour)
	neutral := FeelLonging(now, day, time.Time{}, 0)
	fond := FeelLonging(now, day, time.Time{}, 0.8)
	if !(fond.Missing > neutral.Missing) {
		t.Errorf("a day away: fond %.2f, neutral %.2f — absence should be felt sooner for someone she likes", fond.Missing, neutral.Missing)
	}
	if FeelLonging(now, time.Time{}, now, 1).Missing != 0 {
		t.Error("missed someone she has never talked to")
	}
}

func TestLongingNoticesBeingIgnoredBySomeoneAround(t *testing.T) {
	now := time.Now()
	if !FeelLonging(now, now.Add(-5*time.Hour), now.Add(-5*time.Minute), 0).Neglected {
		t.Error("they are active and have not spoken to her in hours, and she did not notice")
	}
	if FeelLonging(now, now.Add(-5*time.Hour), now.Add(-2*time.Hour), 0).Neglected {
		t.Error("counted someone who is away as ignoring her")
	}
}

// She is not forced to seek attention: the urge comes from how she feels.
func TestUrgeComesFromHowSheFeels(t *testing.T) {
	missed := Longing{Missing: 0.8}
	awake := Drives{Energy: 0.9, Social: 0.5, Interest: 0.5}
	fond := Urge(Reach{Longing: missed, Warmth: 0.8, Drives: awake})
	neutral := Urge(Reach{Longing: missed, Warmth: 0, Drives: awake})
	if !(fond > neutral) {
		t.Errorf("fond %.2f, neutral %.2f", fond, neutral)
	}
	if got := Urge(Reach{Longing: missed, Warmth: 0.8, Irritation: 0.8, Drives: awake}); got != 0 {
		t.Errorf("annoyed with them and still wants their attention: %.2f", got)
	}
}

func eager() Reach {
	return Reach{
		Now: time.Now(), Level: AttentionKeen, Hour: 14,
		Longing: Longing{Missing: 1, Neglected: true}, Warmth: 1,
		Drives: Drives{Energy: 0.9, Social: 1, Interest: 0.5},
	}
}

func TestMayReachRespectsWhatTheyAllowed(t *testing.T) {
	now := time.Now()
	if ok, _ := MayReach(eager(), 0); !ok {
		t.Fatal("refused with every reason to reach out")
	}
	cases := map[string]func(*Reach){
		"not opted in":           func(r *Reach) { r.Level = AttentionOff },
		"already talking to her": func(r *Reach) { r.Engaged = true },
		"day's allowance spent":  func(r *Reach) { r.Today = 3 },
		"inside the cooldown":    func(r *Reach) { r.Last = now.Add(-time.Hour) },
		"night":                  func(r *Reach) { r.Hour = 3 },
		"ignored three times":    func(r *Reach) { r.Unanswered = maxUnanswered },
		"exhausted":              func(r *Reach) { r.Drives.Energy = 0.1 },
		"backoff after being ignored": func(r *Reach) {
			r.Unanswered = 1
			r.Last = now.Add(-5 * time.Hour) // past keen's 4h, short of 8h
		},
	}
	for name, change := range cases {
		r := eager()
		r.Now = now
		change(&r)
		if ok, _ := MayReach(r, 0); ok {
			t.Errorf("%s: reached out anyway", name)
		}
	}
}

// Opting in is permission, not a quota: someone she barely misses mostly
// hears nothing.
func TestMayReachIsRarelyForSomeoneSheBarelyMisses(t *testing.T) {
	r := eager()
	r.Longing, r.Warmth = Longing{Missing: 0.2}, 0
	if ok, urge := MayReach(r, 0.2); ok {
		t.Errorf("reached out to someone she barely misses (urge %.2f)", urge)
	}
}

func TestReachDirectiveSpeaksToHowSheFeels(t *testing.T) {
	r := eager()
	r.Longing.Away = 26 * time.Hour
	if got := ReachDirective("Big M", r); !strings.Contains(got, "talking to other people") || !strings.Contains(got, "You miss them") {
		t.Errorf("fond and ignored: %q", got)
	}
	r.Warmth, r.Longing.Neglected = 0, false
	if got := ReachDirective("Big M", r); !strings.Contains(got, "bored enough") {
		t.Errorf("neutral: %q", got)
	}
}

func TestWantsPeace(t *testing.T) {
	for _, s := range []string{"leave me alone", "stop pinging me pls", "go away domme", "give me some space"} {
		if !WantsPeace(s) {
			t.Errorf("%q not heard as asking her to back off", s)
		}
	}
	for _, s := range []string{"don't leave", "stop the music", "come here"} {
		if WantsPeace(s) {
			t.Errorf("%q heard as asking her to back off", s)
		}
	}
}
