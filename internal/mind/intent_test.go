package mind

import (
	"testing"
	"time"
)

// Ignoring someone's first ever approach is not reticence, it is a bot that
// appears not to work.
func TestDecideAlwaysAnswersAFirstApproach(t *testing.T) {
	a := DefaultAttention()
	s := Situation{Trigger: TriggerNamed, Now: time.Now(), FirstApproach: true}

	if got := Decide(a, s, 0.999); got != OutcomeSpeak {
		t.Errorf("Decide = %q on a first approach with the worst roll, want %q", got, OutcomeSpeak)
	}
}

// Twice running stops reading as character and starts reading as broken.
func TestDecideNeverIgnoresTheSamePersonTwice(t *testing.T) {
	a := DefaultAttention()
	s := Situation{Trigger: TriggerNamed, Now: time.Now(), IgnoredLast: true}

	if got := Decide(a, s, 0.999); got != OutcomeSpeak {
		t.Errorf("Decide = %q after already ignoring them once, want %q", got, OutcomeSpeak)
	}
}

func TestDecideAnswersMentionsFarMoreOftenThanNameDrops(t *testing.T) {
	a := DefaultAttention()
	now := time.Now()

	// A roll between the two chances separates them: a mention gets answered,
	// the same roll on a passing name-drop does not.
	roll := (a.NamedChance + a.MentionChance) / 2

	mention := Situation{Trigger: TriggerMention, Now: now}
	if got := Decide(a, mention, roll); got != OutcomeSpeak {
		t.Errorf("mention: Decide = %q, want %q", got, OutcomeSpeak)
	}

	named := Situation{Trigger: TriggerNamed, Now: now}
	if got := Decide(a, named, roll); got != OutcomeIgnore {
		t.Errorf("name-drop: Decide = %q, want %q", got, OutcomeIgnore)
	}
}

// Dropping out halfway through a back-and-forth reads as a fault, not as
// reticence.
func TestDecideIsMoreWillingMidExchange(t *testing.T) {
	a := DefaultAttention()
	now := time.Now()

	roll := a.MentionChance + a.EngagedBoost/2

	cold := Situation{Trigger: TriggerMention, Now: now}
	if got := Decide(a, cold, roll); got != OutcomeIgnore {
		t.Errorf("cold channel: Decide = %q, want %q", got, OutcomeIgnore)
	}

	engaged := Situation{
		Trigger:     TriggerMention,
		Now:         now,
		LastSpokeAt: now.Add(-time.Minute),
	}
	if got := Decide(a, engaged, roll); got != OutcomeSpeak {
		t.Errorf("mid-exchange: Decide = %q, want %q", got, OutcomeSpeak)
	}
}

// She should not answer every passing use of her name in a conversation she is
// already part of.
func TestDecideIsLessWillingToChaseNameDropsWhileTalking(t *testing.T) {
	a := DefaultAttention()
	now := time.Now()

	roll := a.NamedChance - a.CrowdingPenalty/2

	cold := Situation{Trigger: TriggerNamed, Now: now}
	if got := Decide(a, cold, roll); got != OutcomeSpeak {
		t.Errorf("cold channel: Decide = %q, want %q", got, OutcomeSpeak)
	}

	engaged := Situation{
		Trigger:     TriggerNamed,
		Now:         now,
		LastSpokeAt: now.Add(-time.Minute),
	}
	if got := Decide(a, engaged, roll); got != OutcomeIgnore {
		t.Errorf("already talking: Decide = %q, want %q", got, OutcomeIgnore)
	}
}

func TestDecideRefusesAnUnknownTrigger(t *testing.T) {
	if got := Decide(DefaultAttention(), Situation{Trigger: "something else"}, 0); got != OutcomeIgnore {
		t.Errorf("Decide = %q on an unknown trigger, want %q", got, OutcomeIgnore)
	}
}

// A chance of zero has to mean never, and one has to mean always, or the
// operator knob that disables ignoring entirely does not actually disable it.
func TestDecideRespectsTheExtremes(t *testing.T) {
	never := Attention{MentionChance: 0}
	if got := Decide(never, Situation{Trigger: TriggerMention}, 0); got != OutcomeIgnore {
		t.Errorf("Decide = %q at zero chance, want %q", got, OutcomeIgnore)
	}

	always := Attention{MentionChance: 1}
	if got := Decide(always, Situation{Trigger: TriggerMention}, 0.9999); got != OutcomeSpeak {
		t.Errorf("Decide = %q at full chance, want %q", got, OutcomeSpeak)
	}
}

func TestSaysNameFindsWholeWordsOnly(t *testing.T) {
	names := []string{"Vera", "V"}

	for _, text := range []string{
		"vera should weigh in on this",
		"has anyone asked Vera?",
		"ask V about it",
		"VERA!",
		"(vera)",
	} {
		if !SaysName(text, names) {
			t.Errorf("SaysName(%q) = false, want true", text)
		}
	}

	for _, text := range []string{
		"veranda",
		"several things",
		"overawed",
		"",
		"nothing relevant",
	} {
		if SaysName(text, names) {
			t.Errorf("SaysName(%q) = true, want false", text)
		}
	}
}

// A configured name is routinely several words or carries punctuation —
// "Server Domme", "Server-Domme" — which a regex word boundary handles badly
// and which the operator will certainly write.
func TestSaysNameHandlesMultiWordAndPunctuatedNames(t *testing.T) {
	names := []string{"ServerDomme", "Server-Domme", "Server Domme"}

	for _, text := range []string{
		"has anyone seen Server Domme today",
		"server-domme is quiet",
		"ServerDomme!",
	} {
		if !SaysName(text, names) {
			t.Errorf("SaysName(%q) = false, want true", text)
		}
	}

	if SaysName("serverdommes everywhere", names) {
		t.Error("matched a name glued to a longer word")
	}
}

func TestSaysNameHandlesNothingToMatch(t *testing.T) {
	if SaysName("anything at all", nil) {
		t.Error("an empty name list matched")
	}
	if SaysName("anything at all", []string{"", "   "}) {
		t.Error("blank names matched")
	}
	if SaysName("", []string{"vera"}) {
		t.Error("empty text matched")
	}
}

// Discord renders a mention as the account username, so this is the exact
// shape that reached production as "wrong door. DevBot is not in here".
func TestSaysNameRecognisesTheNameInsideAMention(t *testing.T) {
	if !SaysName("@DevBot test", []string{"DevBot"}) {
		t.Error("did not recognise its own name behind an @")
	}
}

func TestCleanNamesTrimsAndDeduplicates(t *testing.T) {
	got := CleanNames([]string{"ServerDomme", "Server-Domme", " Server Domme ", "", "  ", "serverdomme"})
	want := []string{"ServerDomme", "Server-Domme", "Server Domme"}

	if len(got) != len(want) {
		t.Fatalf("CleanNames = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CleanNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCleanNamesKeepsTheCanonicalNameFirst(t *testing.T) {
	got := CleanNames([]string{"  ", "DevBot", "Dev"})
	if len(got) == 0 || got[0] != "DevBot" {
		t.Errorf("CleanNames = %q, want DevBot first", got)
	}
}

func TestDecideAnswersMostFollowUps(t *testing.T) {
	a := DefaultAttention()
	s := Situation{Trigger: TriggerFollowUp, Now: time.Now(), LastSpokeAt: time.Now().Add(-time.Minute)}

	if got := Decide(a, s, a.FollowUpChance-0.01); got != OutcomeSpeak {
		t.Errorf("Decide = %q just under the chance, want %q", got, OutcomeSpeak)
	}
	// Short of certain on purpose: an open exchange is strong evidence a
	// message is for her, not proof.
	if got := Decide(a, s, a.FollowUpChance+0.01); got != OutcomeIgnore {
		t.Errorf("Decide = %q just over the chance, want %q", got, OutcomeIgnore)
	}
}

// Dropping a direct question mid-conversation does not read as reticence, it
// reads as the bot being broken.
func TestDecideNeverIgnoresADirectApproachMidExchange(t *testing.T) {
	a := DefaultAttention()
	now := time.Now()

	for _, trigger := range []Trigger{TriggerMention, TriggerReply} {
		s := Situation{
			Trigger:     trigger,
			Now:         now,
			LastSpokeAt: now.Add(-time.Minute),
		}
		if got := Decide(a, s, 0.9999); got != OutcomeSpeak {
			t.Errorf("%s mid-exchange: Decide = %q on the worst roll, want %q",
				trigger, got, OutcomeSpeak)
		}
	}
}
