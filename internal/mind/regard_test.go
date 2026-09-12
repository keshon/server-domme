package mind

import (
	"strings"
	"testing"
)

func TestCombineSumsAndClamps(t *testing.T) {
	if got := Combine([]float64{0.3, 0.3}); got != 0.6 {
		t.Errorf("two roles that count should add: %.2f", got)
	}
	if got := Combine([]float64{0.8, 0.8, 0.8}); got != 1 {
		t.Errorf("three favourable roles ran past the top: %.2f", got)
	}
	if got := Combine([]float64{-0.9, -0.9}); got != -1 {
		t.Errorf("ran past the bottom: %.2f", got)
	}
	if got := Combine(nil); got != 0 {
		t.Errorf("no roles = %.2f, want nothing in particular", got)
	}
	// Roles that disagree should cancel, not pick a side.
	if got := Combine([]float64{0.5, -0.5}); got != 0 {
		t.Errorf("opposing roles = %.2f, want 0", got)
	}
}

// A note an operator wrote about their own server says something no scalar
// can, which is the whole reason it exists.
func TestRegardDirectivePrefersTheNote(t *testing.T) {
	note := "a submissive here, speak to them as one"
	got := RegardDirective("cass", note, 0.4)

	if !strings.Contains(got, note) {
		t.Errorf("the note was not used verbatim: %q", got)
	}
	if !strings.Contains(got, "cass") {
		t.Errorf("the directive does not say who it is about: %q", got)
	}
}

func TestRegardDirectiveFallsBackToTheBands(t *testing.T) {
	warm := RegardDirective("cass", "", 0.9)
	cool := RegardDirective("cass", "", -0.9)

	if warm == "" || cool == "" {
		t.Fatal("a strongly set role produced no instruction")
	}
	if warm == cool {
		t.Error("opposite ends produced the same instruction")
	}
}

// A role set to a little of something should say nothing rather than produce a
// sentence nobody meant.
func TestRegardDirectiveIsQuietNearZero(t *testing.T) {
	for _, v := range []float64{0, 0.1, -0.1, 0.2} {
		if got := RegardDirective("cass", "", v); got != "" {
			t.Errorf("regard %.2f produced %q", v, got)
		}
	}
	if got := RegardDirective("", "", 1); got != "" {
		t.Error("produced a directive with nobody to aim it at")
	}
}

// A member who cannot get an answer because of a role they were given has no
// way to tell that from a broken bot.
func TestRegardNudgeStaysSmall(t *testing.T) {
	if got := RegardNudge(-1); got < -0.15 {
		t.Errorf("nudge = %.2f, want it small enough to keep them talking to her", got)
	}
	if RegardNudge(1) <= 0 || RegardNudge(-1) >= 0 {
		t.Error("the nudge does not run both ways")
	}
	if got := RegardNudge(0); got != 0 {
		t.Errorf("no standing moved the odds by %.2f", got)
	}
}
