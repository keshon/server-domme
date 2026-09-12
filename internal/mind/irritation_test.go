package mind

import (
	"strings"
	"testing"
	"time"
)

func TestIrritationWearsOff(t *testing.T) {
	now := time.Now()
	annoyed := Pester(0)

	straightAfter := IrritationNow(annoyed, now, now)
	anHourOn := IrritationNow(annoyed, now.Add(-time.Hour), now)
	nextDay := IrritationNow(annoyed, now.Add(-24*time.Hour), now)

	if !(straightAfter > anHourOn && anHourOn > nextDay) {
		t.Errorf("should fade: %.2f %.2f %.2f", straightAfter, anHourOn, nextDay)
	}
	if nextDay != 0 {
		t.Errorf("still annoyed a day later at %.2f", nextDay)
	}
	if anHourOn <= 0 {
		t.Error("forgot about it within the hour")
	}
}

func TestPesterAccumulatesAndSaturates(t *testing.T) {
	one := Pester(0)
	two := Pester(one)

	if two <= one {
		t.Errorf("pushing twice should be worse than once: %.2f vs %.2f", one, two)
	}
	for i := 0; i < 20; i++ {
		one = Pester(one)
	}
	if one > 1 {
		t.Errorf("irritation ran past 1: %.2f", one)
	}
}

// The whole point of holding this per person: someone else should find her
// ordinary.
func TestIrritationDirectiveNamesThePerson(t *testing.T) {
	sharp := IrritationDirective("cass", 0.9)
	if !strings.Contains(sharp, "cass") {
		t.Errorf("directive does not say who it is about: %q", sharp)
	}

	cool := IrritationDirective("cass", 0.45)
	if cool == "" || cool == sharp {
		t.Errorf("the middle band should read differently: %q vs %q", cool, sharp)
	}

	if got := IrritationDirective("cass", 0.1); got != "" {
		t.Errorf("a trace of annoyance produced %q", got)
	}
	if got := IrritationDirective("", 0.9); got != "" {
		t.Error("produced a directive with nobody to aim it at")
	}
}

// Terser, not silent. A character who stops responding reads as a broken bot
// rather than an annoyed person.
func TestIrritationNudgeIsSmallerThanSilence(t *testing.T) {
	if got := IrritationNudge(1); got < -0.2 {
		t.Errorf("nudge = %.2f, want it modest enough to keep her talking", got)
	}
	if got := IrritationNudge(1); got >= 0 {
		t.Errorf("nudge = %.2f, want it negative", got)
	}
	if got := IrritationNudge(0.1); got != 0 {
		t.Errorf("a trace of annoyance moved the odds by %.2f", got)
	}
}

func TestIrritationNowHandlesNothingStored(t *testing.T) {
	now := time.Now()
	if got := IrritationNow(0, now, now); got != 0 {
		t.Errorf("got %.2f from nothing", got)
	}
	if got := IrritationNow(0.5, time.Time{}, now); got != 0 {
		t.Errorf("got %.2f from an unstamped level", got)
	}
}
