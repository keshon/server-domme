package mind

import (
	"testing"
	"time"
)

// Surprise, not the outcome, is the reward: the same warm answer means more
// from someone she expected nothing from.
func TestSurpriseIsAgainstWhatSheExpected(t *testing.T) {
	fromAStranger := Surprise(PayoffEngaged, 0.2)
	fromAFriend := Surprise(PayoffEngaged, 0.9)
	if fromAStranger <= fromAFriend {
		t.Errorf("warm answer: from someone unexpected %+.2f, from someone reliable %+.2f", fromAStranger, fromAFriend)
	}
	if Surprise(PayoffIgnored, 0.9) >= Surprise(PayoffIgnored, 0.2) {
		t.Error("silence from someone who always answers stung no more than from a stranger")
	}
	if Surprise(PayoffLaughed, 0.5) <= Surprise(PayoffEngaged, 0.5) {
		t.Error("a laugh was no better than an answer")
	}
}

func TestPayoffOfReadsHowItWasTaken(t *testing.T) {
	cases := map[string]Payoff{
		"hahaha":            PayoffLaughed,
		"that was terrible": PayoffPanned,
		"wait, which one?":  PayoffEngaged,
	}
	for content, want := range cases {
		if got := PayoffOf(content); got != want {
			t.Errorf("%q read as %q, want %q", content, got, want)
		}
	}
}

// Learning drives the next pull: unchanged at neutral, so nothing moves until
// she has learned something, and up or down from there.
func TestWelcomeScalesWhatSheStarts(t *testing.T) {
	v := willing(TriggerReturn)
	_, neutral := MayVolunteer(v, 1)
	v.WelcomeShift = 0.4
	_, glad := MayVolunteer(v, 1)
	v.WelcomeShift = -0.4
	_, cold := MayVolunteer(v, 1)
	if !(glad > neutral && neutral > cold) {
		t.Errorf("glad %.2f, neutral %.2f, cold %.2f", glad, neutral, cold)
	}
	if WelcomeShift(welcomeNeutral) != 0 {
		t.Error("neutral welcome shifted the pull")
	}

	a := clipped()
	_, before := MayAddAfterthought(a, 1)
	a.WelcomeShift = -0.4
	if _, after := MayAddAfterthought(a, 1); after >= before {
		t.Error("a second thought was as likely with someone who never takes them up")
	}
}

// Being taken up teaches her she is welcome; being let drop, that she is
// less so — each a little, since a remark in a room says less than reaching
// out does.
func TestPayoffTeachesWelcome(t *testing.T) {
	now := time.Now()
	up := Bond{}.Apply(PayoffEvent(PayoffEngaged), now)
	down := Bond{}.Apply(PayoffEvent(PayoffIgnored), now)
	_, _, w0 := Bond{}.Now(now)
	_, _, wUp := up.Now(now)
	_, _, wDown := down.Now(now)
	if !(wUp > w0 && wDown < w0) {
		t.Errorf("welcome after being taken up %.2f, let drop %.2f, from %.2f", wUp, wDown, w0)
	}
	if Appraise(EventInitiativeTaken).Welcome >= Appraise(EventReachAnswered).Welcome {
		t.Error("a remark taken up taught as much as a reach answered")
	}
}
