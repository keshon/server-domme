package mind

import (
	"strings"
	"testing"
	"time"
)

func TestFamiliarityFollowsMessageCount(t *testing.T) {
	cases := []struct {
		messages int
		want     Familiarity
	}{
		{0, FamiliarityNewcomer},
		{newcomerBelow - 1, FamiliarityNewcomer},
		{newcomerBelow, FamiliarityKnown},
		{regularAbove, FamiliarityKnown},
		{regularAbove + 1, FamiliarityRegular},
	}
	for _, tc := range cases {
		got := Acquaintance{Messages: tc.messages}.Familiarity()
		if got != tc.want {
			t.Errorf("Familiarity(%d messages) = %q, want %q", tc.messages, got, tc.want)
		}
	}
}

func TestAwayForReportsOnlyARealAbsence(t *testing.T) {
	now := time.Now()

	midConversation := Acquaintance{PrevSeen: now.Add(-time.Minute), LastSeen: now}
	if got := midConversation.AwayFor(); got != 0 {
		t.Errorf("AwayFor(mid-conversation) = %v, want 0", got)
	}

	returning := Acquaintance{PrevSeen: now.Add(-30 * 24 * time.Hour), LastSeen: now}
	if got := returning.AwayFor(); got == 0 {
		t.Error("AwayFor(a month away) = 0, want the absence reported")
	}

	unseen := Acquaintance{}
	if got := unseen.AwayFor(); got != 0 {
		t.Errorf("AwayFor(never seen) = %v, want 0", got)
	}

	// Someone seen for the very first time has no previous sighting, so there
	// is no absence to report even though the gap looks infinite.
	brandNew := Acquaintance{LastSeen: now}
	if got := brandNew.AwayFor(); got != 0 {
		t.Errorf("AwayFor(first ever sighting) = %v, want 0", got)
	}
}

// An empty field rendered as a blank reads to a model as "this is unknown" and
// invites it to make something up.
func TestGroundingRenderSkipsEmptyFields(t *testing.T) {
	got := Grounding{ChannelName: "general", Now: time.Time{}}.Render()

	if strings.Contains(got, "The server is") {
		t.Errorf("rendered an empty guild name:\n%s", got)
	}
	if strings.Contains(got, "It is .") {
		t.Errorf("rendered an empty time of day:\n%s", got)
	}
	if !strings.Contains(got, "#general") {
		t.Errorf("dropped the channel it does know:\n%s", got)
	}
}

func TestGroundingRenderNotesAReturnAfterAbsence(t *testing.T) {
	now := time.Now()
	g := Grounding{
		Now: now,
		Present: []Acquaintance{
			{
				Username: "ghost", Messages: 200,
				PrevSeen: now.Add(-60 * 24 * time.Hour),
				LastSeen: now,
			},
		},
	}
	got := g.Render()
	if !strings.Contains(got, "back after") {
		t.Errorf("absence not mentioned:\n%s", got)
	}
}

func TestTimeOfDayNamesThePartOfDay(t *testing.T) {
	cases := []struct {
		hour int
		want string
	}{
		{3, "the middle of the night"},
		{9, "morning"},
		{15, "afternoon"},
		{20, "evening"},
		{23, "late evening"},
	}
	for _, tc := range cases {
		at := time.Date(2026, 3, 1, tc.hour, 0, 0, 0, time.UTC)
		if got := timeOfDay(at); got != tc.want {
			t.Errorf("timeOfDay(%02d:00) = %q, want %q", tc.hour, got, tc.want)
		}
	}
}

// The failure this prevents reached production: a bot configured as "Dev" but
// named DevBot in Discord read "@DevBot test" and answered "wrong door. DevBot
// is not in here". A mention expands to the account username, so the prompt
// has to say that every spelling is her.
func TestGroundingRenderStatesWhoSheIs(t *testing.T) {
	g := Grounding{
		SelfName:    "DevBot",
		SelfAliases: []string{"Dev", "ServerDomme"},
		Now:         time.Now(),
	}
	got := g.Render()

	for _, want := range []string{"DevBot", "Dev", "ServerDomme", "means you"} {
		if !strings.Contains(got, want) {
			t.Errorf("identity block is missing %q:\n%s", want, got)
		}
	}
}

func TestGroundingRenderSkipsIdentityWhenUnknown(t *testing.T) {
	got := Grounding{ChannelName: "general"}.Render()
	if strings.Contains(got, "Your name here") {
		t.Errorf("rendered an empty identity block:\n%s", got)
	}
}

// The identity has to come before the surroundings: it is what everything
// else in the conversation is interpreted against.
func TestGroundingRendersIdentityFirst(t *testing.T) {
	g := Grounding{SelfName: "DevBot", GuildName: "The Parlour", Now: time.Now()}
	got := g.Render()

	if strings.Index(got, "DevBot") > strings.Index(got, "The Parlour") {
		t.Errorf("identity is rendered after the surroundings:\n%s", got)
	}
}
