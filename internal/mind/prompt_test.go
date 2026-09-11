package mind

import (
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/ai"
)

func testCharacter() *Character {
	return &Character{
		Name:    "Vera",
		Persona: "You are Vera, and you have been here a long time.",
		Avoid:   []string{"Anything involving minors."},
		Examples: []Exchange{
			{User: "you around?", Assistant: "unfortunately"},
			{User: "help me", Assistant: "with what, specifically"},
		},
	}
}

// Examples must reach the model as conversation turns. Quoting them inside the
// system message is the obvious-looking simplification and it flattens the
// voice, which is the whole reason they are separate messages.
func TestBuildReplaysExamplesAsRealTurns(t *testing.T) {
	msgs := Build(testCharacter(), Grounding{}, nil, DefaultBudget())

	if msgs[0].Role != ai.RoleSystem {
		t.Fatalf("first message role = %q, want system", msgs[0].Role)
	}
	if strings.Contains(msgs[0].Content, "unfortunately") {
		t.Error("examples were folded into the system message instead of being replayed as turns")
	}

	want := []struct{ role, content string }{
		{ai.RoleUser, "you around?"},
		{ai.RoleAssistant, "unfortunately"},
		{ai.RoleUser, "help me"},
		{ai.RoleAssistant, "with what, specifically"},
	}
	for i, w := range want {
		got := msgs[i+1]
		if got.Role != w.role || got.Content != w.content {
			t.Errorf("message %d = {%q, %q}, want {%q, %q}",
				i+1, got.Role, got.Content, w.role, w.content)
		}
	}
}

func TestBuildPutsPersonaAndLimitsInTheSystemMessage(t *testing.T) {
	msgs := Build(testCharacter(), Grounding{}, nil, DefaultBudget())
	sys := msgs[0].Content

	for _, want := range []string{"You are Vera", "Anything involving minors.", "Write only your own next message"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system message is missing %q:\n%s", want, sys)
		}
	}
}

func TestBuildCapsTheNumberOfExamples(t *testing.T) {
	c := testCharacter()
	b := DefaultBudget()
	b.MaxExamples = 1

	msgs := Build(c, Grounding{}, nil, b)
	if len(msgs) != 3 {
		t.Fatalf("built %d messages, want system plus one exchange", len(msgs))
	}
}

// A channel has several people in it and the wire format has one user role for
// all of them; without names the model answers a composite of everyone.
func TestBuildLabelsSpeakersButNotTheBot(t *testing.T) {
	now := time.Now()
	turns := []Turn{
		{Username: "ann", Content: "who broke the build", At: now},
		{Content: "not me", At: now, FromBot: true},
		{Username: "bob", Content: "probably ann", At: now},
	}

	msgs := Build(testCharacter(), Grounding{}, turns, DefaultBudget())
	tail := msgs[len(msgs)-3:]

	if tail[0].Content != "ann: who broke the build" {
		t.Errorf("user turn = %q, want it labelled", tail[0].Content)
	}
	if tail[1].Role != ai.RoleAssistant || tail[1].Content != "not me" {
		t.Errorf("bot turn = {%q, %q}, want an unlabelled assistant turn", tail[1].Role, tail[1].Content)
	}
	if tail[2].Content != "bob: probably ann" {
		t.Errorf("user turn = %q, want it labelled", tail[2].Content)
	}
}

// Trimming has to drop the oldest turns: a budget that cut from the end would
// leave the model replying to something nobody said most recently.
func TestBuildTrimsHistoryFromTheOldestEnd(t *testing.T) {
	now := time.Now()
	var turns []Turn
	for i := 0; i < 50; i++ {
		turns = append(turns, Turn{
			Username: "ann",
			Content:  strings.Repeat("x", 100),
			At:       now.Add(time.Duration(i) * time.Second),
		})
	}
	turns = append(turns, Turn{Username: "ann", Content: "THE LAST THING SAID", At: now.Add(time.Hour)})

	b := DefaultBudget()
	b.MaxHistoryChars = 500
	msgs := Build(testCharacter(), Grounding{}, turns, b)

	last := msgs[len(msgs)-1].Content
	if !strings.Contains(last, "THE LAST THING SAID") {
		t.Fatalf("most recent turn was trimmed away; last message is %q", last)
	}
	if len(msgs) > 12 {
		t.Errorf("history budget did not bite: %d messages", len(msgs))
	}
}

func TestBuildRendersGroundingIntoTheSystemMessage(t *testing.T) {
	g := Grounding{
		GuildName:    "The Parlour",
		ChannelName:  "general",
		ChannelTopic: "anything goes",
		Brief:        "a small server for people who already know each other",
		Now:          time.Date(2026, 3, 1, 21, 0, 0, 0, time.UTC),
		Present: []Acquaintance{
			{Username: "ann", Messages: 400, LastSeen: time.Date(2026, 3, 1, 20, 0, 0, 0, time.UTC)},
			{Username: "newbie", Messages: 1},
		},
	}

	sys := Build(testCharacter(), g, nil, DefaultBudget())[0].Content

	for _, want := range []string{
		"The Parlour",
		"#general",
		"anything goes",
		"already know each other",
		"evening",
		string(FamiliarityRegular),
		string(FamiliarityNewcomer),
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("system message is missing %q:\n%s", want, sys)
		}
	}
}

// Placed among the surroundings this was ignored in testing: the model
// answered the question and said nothing about the delay. It has to be the
// last thing in the system message, with the other instructions about how to
// answer.
func TestBuildPutsTheLateNoteLast(t *testing.T) {
	g := Grounding{
		GuildName:      "The Parlour",
		Now:            time.Now(),
		AnsweringAfter: 8 * time.Minute,
	}

	msgs := Build(testCharacter(), g, nil, DefaultBudget())
	last := msgs[len(msgs)-1]

	note := g.LateNote()
	if note == "" {
		t.Fatal("LateNote is empty for a delayed reply")
	}
	if last.Role != ai.RoleSystem || last.Content != note {
		t.Errorf("last message = {%q, %q}, want the late note last", last.Role, last.Content)
	}
	if !strings.Contains(last.Content, "8 minutes") {
		t.Errorf("the delay is not stated: %q", last.Content)
	}
}

func TestBuildOmitsTheLateNoteForAPromptReply(t *testing.T) {
	msgs := Build(testCharacter(), Grounding{Now: time.Now()}, nil, DefaultBudget())
	for _, m := range msgs {
		if strings.Contains(m.Content, "only getting to this now") {
			t.Errorf("a prompt reply carries a lateness instruction: %q", m.Content)
		}
	}
}
