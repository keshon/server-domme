package mind

import (
	"testing"
	"time"
)

// Someone who has only just opted in has not shown whether they want her.
func TestWelcomeStartsNeutral(t *testing.T) {
	if _, _, welcome := (Bond{}).Now(time.Now()); welcome != welcomeNeutral {
		t.Errorf("welcome before anything happened = %.2f, want neutral", welcome)
	}
}

func TestWelcomeDriftsBackToNeutral(t *testing.T) {
	now := time.Now()
	b := Bond{Welcome: 1, WelcomeAt: now.Add(-welcomeHalflife)}
	if _, _, w := b.Now(now); w < 0.74 || w > 0.76 {
		t.Errorf("one half-life after full welcome: %.2f, want halfway back to neutral", w)
	}
}

// An event only restamps what it moves: being asked to back off must not
// reset how long tension has been fading.
func TestApplyOnlyTouchesWhatTheEventMoves(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	b := Bond{Tension: 0.8, TensionAt: earlier}.Apply(EventAskedForPeace, now)
	if !b.TensionAt.Equal(earlier) || b.Tension != 0.8 {
		t.Errorf("a welcome event touched tension: %+v", b)
	}
	if _, _, w := b.Now(now); w >= welcomeNeutral {
		t.Errorf("asked to back off and welcome is %.2f", w)
	}
}

func TestEveryNamedEventMovesSomething(t *testing.T) {
	for e, s := range appraisals {
		if s == (Shift{}) {
			t.Errorf("%q is in the table and moves nothing", e)
		}
	}
}

func TestConversationEventsAttributeOnlyOneToOne(t *testing.T) {
	if ConversationEvent(ToneHostile, false) != "" || ConversationEvent(ToneTense, false) != "" {
		t.Error("a group row was held against someone in it")
	}
	if ConversationEvent(ToneWarm, false) != EventWarmGroup {
		t.Error("a warm group conversation moved nobody")
	}
	if ConversationEvent(ToneHostile, true) != EventHostileAlone {
		t.Error("a hostile one-to-one moved nobody")
	}
}
