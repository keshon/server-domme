package mind

import (
	"strings"
	"testing"
	"time"
)

// willing is a volunteer that passes every gate, so each test can break
// exactly one and show that gate alone stops her.
// willing is a volunteer with every gate open, drawn as strongly as the old
// constants did — 0.6 for a returning regular, 0.35 for a subject — so the
// gate tests describe the same situations they always did.
func willing(trigger Trigger) Volunteer {
	pull := 0.35
	if trigger == TriggerReturn {
		pull = 0.6
	}
	return Volunteer{
		Pull:          pull,
		Trigger:       trigger,
		Now:           time.Now(),
		Enabled:       true,
		EngagedWindow: 3 * time.Minute,
		UserID:        "u1",
	}
}

func TestMayVolunteerWhenEveryGatePasses(t *testing.T) {
	for _, trigger := range []Trigger{TriggerReturn, TriggerRecall} {
		if !volunteers(willing(trigger), 0) {
			t.Errorf("%s: refused with every gate open", trigger)
		}
	}
}

func TestMayVolunteerGates(t *testing.T) {
	now := time.Now()
	cases := map[string]func(*Volunteer){
		"channel not opted in": func(v *Volunteer) { v.Enabled = false },
		"daily budget spent":   func(v *Volunteer) { v.Today = VolunteerDailyMax },
		// Already talking: what she says next is a reply, not a volunteer.
		"already in the conversation": func(v *Volunteer) { v.LastSpokeAt = now.Add(-time.Minute) },
		"too tired":                   func(v *Volunteer) { v.Drives = Drives{Energy: 0.1, Arousal: 0.5} },
		"not a volunteered trigger":   func(v *Volunteer) { v.Trigger = TriggerMention },
	}

	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			v := willing(TriggerReturn)
			v.Now = now
			breakIt(&v)
			if volunteers(v, 0) {
				t.Error("spoke anyway")
			}
		})
	}
}

// Two other people mid-exchange is not a moment to cut in with something she
// remembered — the same barging "about" was split out to prevent.
func TestMayVolunteerDoesNotCutIntoAnExchange(t *testing.T) {
	now := time.Now()
	v := willing(TriggerRecall)
	v.Now = now
	v.Turns = []Turn{
		{UserID: "u2", Content: "no, the pins stay", At: now.Add(-30 * time.Second)},
		{UserID: "u3", Content: "they never stay", At: now.Add(-10 * time.Second)},
		{UserID: "u1", Content: "the purge rules again?", At: now},
	}
	if volunteers(v, 0) {
		t.Error("cut into two other people talking")
	}

	// The same room, one other voice: the person who prompted it is talking
	// to one other person, which is not a closed exchange.
	v.Turns = v.Turns[1:]
	if !volunteers(v, 0) {
		t.Error("refused a room with only one other voice in it")
	}
}

// A regular coming back is an event whoever else is talking; the busy-room
// check is for bringing up a memory, not for noticing a person.
func TestMayVolunteerGreetsAReturnEvenInABusyRoom(t *testing.T) {
	now := time.Now()
	v := willing(TriggerReturn)
	v.Now = now
	v.Turns = []Turn{
		{UserID: "u2", Content: "a", At: now.Add(-20 * time.Second)},
		{UserID: "u3", Content: "b", At: now.Add(-10 * time.Second)},
	}
	if !volunteers(v, 0) {
		t.Error("ignored a returning regular because the room was busy")
	}
}

// Caps rather than probabilities: the roll only ever says no to a remark that
// was already allowed.
func TestMayVolunteerRollOnlyRefines(t *testing.T) {
	if volunteers(willing(TriggerRecall), 0.99) {
		t.Error("a near-certain roll still spoke at recall's modest odds")
	}
	if volunteers(willing(TriggerReturn), 0.99) {
		t.Error("a near-certain roll still spoke at return's odds")
	}
}

func TestRelevantFindsTheSubjectTheRoomIsOn(t *testing.T) {
	now := time.Now()
	memories := []Memory{
		{At: now.Add(-3 * 24 * time.Hour), Gist: "argument about the purge rules", Detail: "pins and purges"},
		{At: now.Add(-3 * 24 * time.Hour), Gist: "someone shared a film", Detail: "nobody watched it"},
	}

	got, _, ok := Relevant(memories, now, Keywords("are the purge rules changing"))
	if !ok {
		t.Fatal("found nothing for a subject she remembers")
	}
	if !strings.Contains(got.Gist, "purge") {
		t.Errorf("picked %q", got.Gist)
	}

	if _, _, ok := Relevant(memories, now, Keywords("anyone for chess")); ok {
		t.Error("found a memory for a subject she has none of")
	}
}

// Without a minimum age the freshest memory always matches the room it came
// from, and she would bring up the last hour as though it were history.
func TestRelevantIgnoresTheConversationSheIsIn(t *testing.T) {
	now := time.Now()
	fresh := []Memory{{At: now.Add(-20 * time.Minute), Gist: "argument about the purge rules"}}

	if _, _, ok := Relevant(fresh, now, Keywords("the purge rules")); ok {
		t.Error("recalled a conversation from twenty minutes ago")
	}
}

func TestVolunteeredSeparatesStartingFromBeingAsked(t *testing.T) {
	for _, t2 := range []Trigger{TriggerReturn, TriggerRecall} {
		if !Volunteered(t2) {
			t.Errorf("%s should count as volunteered", t2)
		}
	}
	for _, t2 := range []Trigger{TriggerMention, TriggerReply, TriggerNamed, TriggerAbout, TriggerFollowUp} {
		if Volunteered(t2) {
			t.Errorf("%s is an answer, not a volunteer", t2)
		}
	}
}

func TestVolunteerDirectivesSayWhyAndKeepItShort(t *testing.T) {
	back := ReturnDirective("cass", 20*24*time.Hour)
	if !strings.Contains(back, "cass") || !strings.Contains(back, "One line") || !strings.Contains(back, "do not know what happened") {
		t.Errorf("return directive: %q", back)
	}

	recall := RecallDirective("the row about pins")
	if !strings.Contains(recall, "the row about pins") || !strings.Contains(recall, "Leave their question") {
		t.Errorf("recall directive: %q", recall)
	}
}

func volunteers(v Volunteer, roll float64) bool {
	ok, _ := MayVolunteer(v, roll)
	return ok
}
