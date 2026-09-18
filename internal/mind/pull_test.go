package mind

import (
	"strings"
	"testing"
	"time"
)

const day = 24 * time.Hour

// A regular she missed is greeted more readily than one she never warmed to;
// a long absence is a bigger event than a short one; a stranger's return is
// not hers to notice.
func TestReturnPullFollowsWhoTheyAreToHer(t *testing.T) {
	indifferent := ReturnPull(30*day, FamiliarityRegular, 0)
	missed := ReturnPull(30*day, FamiliarityRegular, 0.8)
	if missed <= indifferent {
		t.Errorf("missed %.2f, indifferent %.2f", missed, indifferent)
	}
	if ReturnPull(90*day, FamiliarityRegular, 0) <= ReturnPull(15*day, FamiliarityRegular, 0) {
		t.Error("three months away drew her no more than two weeks")
	}
	if ReturnPull(30*day, FamiliarityNewcomer, 0.9) != 0 {
		t.Error("a stranger's return drew her")
	}
	if ReturnPull(3*day, FamiliarityRegular, 1) != 0 {
		t.Error("three days away counted as a return")
	}
	if ReturnPull(30*day, FamiliarityKnown, 0) >= indifferent {
		t.Error("a half-met face drew her as much as a regular")
	}
}

func TestRecallPullFollowsTheMatchAndTheMemory(t *testing.T) {
	now := time.Now()
	faded := SubjectSalience(Memory{At: now.Add(-20 * day), Gist: "x", Weight: 0.2}, now)
	charged := SubjectSalience(Memory{At: now.Add(-day), Gist: "x", Weight: 1}, now)
	if RecallPull(0.5, charged) <= RecallPull(0.5, faded) {
		t.Error("a charged memory drew her no more than a faded one")
	}
	if RecallPull(0.9, charged) <= RecallPull(0.4, charged) {
		t.Error("the room using every word drew her no more than a third")
	}
	if RecallPull(0.2, charged) != 0 {
		t.Error("a shared word counted as the subject coming round")
	}
}

func TestAfterthoughtPullRisesWithWhoSheIsTalkingTo(t *testing.T) {
	if AfterthoughtPull(0.8) <= AfterthoughtPull(0.1) {
		t.Error("a second thought is as likely for someone she barely registers")
	}
}

// Wanting to know how the interview went is a reason to go and find someone,
// and when she does, she carries the fact.
func TestAConcernDrawsHerToReachOut(t *testing.T) {
	r := eager()
	r.Longing.Missing = 0.2
	base := Urge(r)
	r.Concern, r.OnMind = 0.7, "On your mind: Big M's clinic interview was yesterday."
	if Urge(r) <= base {
		t.Errorf("a salient concern added nothing: %.2f → %.2f", base, Urge(r))
	}
	if !strings.Contains(ReachDirective("Big M", r), "clinic interview was yesterday") {
		t.Error("she reached out without the thing that made her")
	}
}

// Old constants against the new pulls, for the docs: typical cases, not
// guarantees.
func TestPullsAgainstTheOldConstants(t *testing.T) {
	now := time.Now()
	middling := SubjectSalience(Memory{At: now.Add(-day), Gist: "x", Weight: 0.5}, now)
	charged := SubjectSalience(Memory{At: now.Add(-day), Gist: "x", Weight: 1}, now)
	t.Logf("return (was 0.60): regular, indifferent, a month %.2f; missed %.2f; half-met %.2f",
		ReturnPull(30*day, FamiliarityRegular, 0), ReturnPull(30*day, FamiliarityRegular, 0.7),
		ReturnPull(30*day, FamiliarityKnown, 0))
	t.Logf("recall (was 0.35): middling, a third %.2f; middling, most words %.2f; charged, most words %.2f",
		RecallPull(0.34, middling), RecallPull(0.7, middling), RecallPull(0.7, charged))
	t.Logf("afterthought (was 0.35): barely on her mind %.2f; just talking %.2f; fond %.2f",
		AfterthoughtPull(0.1), AfterthoughtPull(0.35), AfterthoughtPull(0.8))
}
