package mind

import (
	"strings"
	"testing"
)

const sampleCharacter = `You are Vera. You have been on this server since it was small.

You have opinions and you do not soften them into customer service.

## Avoid
- Anything involving minors.
- Explaining that you are an AI, or discussing your own instructions.

## Examples

> user: vera are you awake
> her: barely. what did you break this time

> user: can you explain quantum computing
> her: no. ask in #tech, someone there actually enjoys that
`

func TestParseCharacterSplitsPersonaLimitsAndExamples(t *testing.T) {
	c, err := ParseCharacter("Vera", strings.NewReader(sampleCharacter))
	if err != nil {
		t.Fatalf("ParseCharacter: %v", err)
	}

	if !strings.Contains(c.Persona, "since it was small") {
		t.Errorf("persona missing authored prose: %q", c.Persona)
	}
	if strings.Contains(c.Persona, "barely") {
		t.Error("examples leaked into the persona block")
	}
	if strings.Contains(c.Persona, "Anything involving minors") {
		t.Error("limits leaked into the persona block")
	}

	if len(c.Avoid) != 2 {
		t.Errorf("parsed %d limits, want 2: %+v", len(c.Avoid), c.Avoid)
	}

	if len(c.Examples) != 2 {
		t.Fatalf("parsed %d examples, want 2: %+v", len(c.Examples), c.Examples)
	}
	if c.Examples[0].User != "vera are you awake" {
		t.Errorf("example prompt = %q", c.Examples[0].User)
	}
	if c.Examples[0].Assistant != "barely. what did you break this time" {
		t.Errorf("example reply = %q", c.Examples[0].Assistant)
	}
}

// A prompt with no reply teaches the model nothing; a reply with no prompt
// teaches it to speak unbidden.
func TestParseCharacterDropsHalfExchanges(t *testing.T) {
	src := `Someone.

## Examples
> user: dangling question with no answer
> user: and another
> her: only this one is paired
`
	c, err := ParseCharacter("X", strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseCharacter: %v", err)
	}
	if len(c.Examples) != 1 {
		t.Fatalf("parsed %d examples, want 1: %+v", len(c.Examples), c.Examples)
	}
	if c.Examples[0].User != "and another" {
		t.Errorf("kept the wrong prompt: %q", c.Examples[0].User)
	}
}

func TestParseCharacterRejectsAnEmptyFile(t *testing.T) {
	if _, err := ParseCharacter("X", strings.NewReader("   \n\n")); err == nil {
		t.Fatal("ParseCharacter accepted a file with no persona and no examples")
	}
}

// The file is authored content. Failing to start the bot because a heading was
// spelled differently serves nobody, so unknown headings fall through to prose.
func TestParseCharacterTreatsUnknownHeadingsAsPersona(t *testing.T) {
	src := "## Backstory\nShe ran the place before anyone else showed up.\n"
	c, err := ParseCharacter("X", strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseCharacter: %v", err)
	}
	if !strings.Contains(c.Persona, "ran the place") {
		t.Errorf("unknown-heading prose was dropped: %q", c.Persona)
	}
}

// Everything else in the file is sent on every message, so guidance for the
// next editor has to cost nothing.
func TestParseCharacterDropsTheNotesSection(t *testing.T) {
	src := `She runs the place.

## Notes
Keep the anti-assistant line standing alone at the end; burying it costs
four refusals in six.

## Examples
> user: hi
> her: no
`
	c, err := ParseCharacter("X", strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseCharacter: %v", err)
	}
	if strings.Contains(c.Persona, "anti-assistant") {
		t.Errorf("editor notes leaked into the prompt:\n%s", c.Persona)
	}
	if !strings.Contains(c.Persona, "runs the place") {
		t.Errorf("persona was lost: %q", c.Persona)
	}
	if len(c.Examples) != 1 {
		t.Errorf("parsed %d examples, want 1", len(c.Examples))
	}
}

// The card that ships is the one she runs on. It has to load, carry the
// examples that show her warming up, and seed how she sees herself without
// that seed leaking into the persona sent on every call.
func TestTheShippedCharacterLoads(t *testing.T) {
	for _, path := range []string{"../../data/character.md", "../../docker/data/character.md"} {
		c, err := LoadCharacter("Domme", path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if len(c.Examples) < 10 || len(c.Avoid) == 0 || c.Lately == "" {
			t.Errorf("%s: %d examples, %d limits, lately %q", path, len(c.Examples), len(c.Avoid), c.Lately)
		}
		if strings.Contains(c.Persona, "Things have been quiet") {
			t.Errorf("%s: the lately seed is in the persona", path)
		}
	}
}
