package ask

import (
	"strings"
	"testing"
)

const (
	asker  = "A"
	target = "B"
)

// The status strings the handler writes have to be readable back as state by
// the next press, since the message is the only record. These build the same
// descriptions Component produces, so a reworded status breaks these tests
// rather than production.
func acceptedDesc() string { return "<@B> " + markerAccepted + " <@A>'s **DM** request." }
func declinedDesc() string { return "<@B> " + markerDeclined + " <@A>'s **DM** request." }
func revokedDesc() string  { return "<@A> " + markerRevoked + " their **DM** request to <@B>." }
func closedDesc() string   { return "<@A> " + markerClosed + " the **DM** conversation with <@B>." }
func pendingDesc() string  { return "<@A> wants to **DM** <@B>" }

func TestStateOf(t *testing.T) {
	cases := []struct {
		name string
		desc string
		want askState
	}{
		{"pending state", pendingDesc(), statePending},
		{"accepted", acceptedDesc(), stateActive},
		{"declined", declinedDesc(), stateDeclined},
		{"revoked", revokedDesc(), stateFinished},
		{"closed", closedDesc(), stateFinished},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stateOf(tc.desc); got != tc.want {
				t.Errorf("stateOf(%q) = %v, want %v", tc.desc, got, tc.want)
			}
		})
	}
}

func TestRefusalPermissions(t *testing.T) {
	cases := []struct {
		name    string
		action  string
		state   askState
		clicker string
		allowed bool
	}{
		// Only the target answers, and only while pending.
		{"target accepts pending", actionAccept, statePending, target, true},
		{"asker cannot accept", actionAccept, statePending, asker, false},
		{"target cannot accept twice", actionAccept, stateActive, target, false},
		{"target denies pending", actionDeny, statePending, target, true},
		{"asker cannot deny", actionDeny, statePending, asker, false},

		// Revoke is the asker's, and only before an answer exists.
		{"asker revokes pending", actionRevoke, statePending, asker, true},
		{"target cannot revoke", actionRevoke, statePending, target, false},
		{"asker cannot revoke accepted", actionRevoke, stateActive, asker, false},
		{"asker cannot revoke declined", actionRevoke, stateDeclined, asker, false},

		// Close belongs to both, and only once a conversation exists. This is
		// the whole point of the feature: the asker can end it too.
		{"asker closes active", actionClose, stateActive, asker, true},
		{"target closes active", actionClose, stateActive, target, true},
		{"cannot close pending", actionClose, statePending, target, false},
		{"cannot close declined", actionClose, stateDeclined, target, false},
		{"cannot close twice", actionClose, stateFinished, target, false},

		{"unknown action", "wat", statePending, target, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := refusal(tc.action, tc.state, tc.clicker, asker, target)
			if allowed := msg == ""; allowed != tc.allowed {
				t.Errorf("refusal(%s, %v, %s) = %q; allowed=%v, want %v",
					tc.action, tc.state, tc.clicker, msg, allowed, tc.allowed)
			}
		})
	}
}

// TestLegacyRevokeOnAcceptedBecomesClose covers the buttons already sitting in
// channels: they carry :revoke on accepted requests, where it meant "end the
// agreement". Those ids cannot be repointed, so the handler translates them —
// and both parties must be able to use them, not just the target.
func TestLegacyRevokeOnAcceptedBecomesClose(t *testing.T) {
	state := stateOf(acceptedDesc())
	action := translateLegacyAction(actionRevoke, state)
	if action != actionClose {
		t.Fatalf("legacy revoke on an accepted request = %q, want %q", action, actionClose)
	}
	for _, clicker := range []string{asker, target} {
		if msg := refusal(action, state, clicker, asker, target); msg != "" {
			t.Errorf("clicker %s refused on translated close: %q", clicker, msg)
		}
	}
}

// Revoke on a request nobody has answered still means revoke; only the accepted
// case is a legacy close.
func TestTranslateLeavesLiveActionsAlone(t *testing.T) {
	cases := []struct {
		action string
		state  askState
	}{
		{actionRevoke, statePending},
		{actionAccept, statePending},
		{actionDeny, statePending},
		{actionClose, stateActive},
		{actionRevoke, stateDeclined},
	}
	for _, tc := range cases {
		if got := translateLegacyAction(tc.action, tc.state); got != tc.action {
			t.Errorf("translateLegacyAction(%q, %v) = %q, want unchanged", tc.action, tc.state, got)
		}
	}
}

// A denied request used to keep a Revoke button that emitted "revoked their
// agreement" — after a refusal, where no agreement had ever existed. Denial is
// terminal now, so every action bounces off it.
func TestDeclinedIsTerminal(t *testing.T) {
	for _, action := range []string{actionAccept, actionDeny, actionRevoke, actionClose} {
		for _, clicker := range []string{asker, target} {
			if msg := refusal(action, stateDeclined, clicker, asker, target); msg == "" {
				t.Errorf("action %q by %s was allowed on a declined request", action, clicker)
			}
		}
	}
}

func TestOtherParty(t *testing.T) {
	if got := otherParty(asker, asker, target); got != target {
		t.Errorf("otherParty(asker) = %q, want %q", got, target)
	}
	if got := otherParty(target, asker, target); got != asker {
		t.Errorf("otherParty(target) = %q, want %q", got, asker)
	}
}

// The reason lives only in the description, so it has to survive being carried
// from one status to the next. It previously did not: the first transition
// rewrote the marker to "Reason was:", which the next lookup could not find.
func TestReasonSurvivesRepeatedTransitions(t *testing.T) {
	desc := "<@A> wants to **DM** <@B>" + formatReason("please and thank you")

	first := "<@B> " + markerAccepted + " <@A>'s **DM** request." + carryReason(desc)
	if !strings.Contains(first, "please and thank you") {
		t.Fatalf("reason lost on the first transition: %q", first)
	}

	second := "<@A> " + markerClosed + " the **DM** conversation with <@B>." + carryReason(first)
	if !strings.Contains(second, "please and thank you") {
		t.Errorf("reason lost on the second transition: %q", second)
	}
}

func TestCarryReasonWithoutOne(t *testing.T) {
	if got := carryReason(pendingDesc()); got != "" {
		t.Errorf("carryReason on a reasonless request = %q, want empty", got)
	}
	if got := formatReason(""); got != "" {
		t.Errorf("formatReason(\"\") = %q, want empty", got)
	}
}
