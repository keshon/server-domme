package mind

import (
	"strings"
	"testing"
)

func TestSpeechStyleOnlySpeaksForDialsSetAwayFromTheMiddle(t *testing.T) {
	if got := DefaultSpeechStyle().Directives(); len(got) != 0 {
		t.Errorf("an unremarkable temperament produced %q", got)
	}
	// Unset, not "cold, humourless and meek".
	if got := (SpeechStyle{}).Directives(); len(got) != 0 {
		t.Errorf("the zero value produced %q", got)
	}

	one := DefaultSpeechStyle()
	one.Sarcasm = 0.9
	if got := one.Directives(); len(got) != 1 {
		t.Errorf("one dial set produced %d directives: %q", len(got), got)
	}
}

func TestSpeechStyleDirectivesInstructRatherThanDescribe(t *testing.T) {
	terse := DefaultSpeechStyle()
	terse.Verbosity = 0.1
	terse.Formality = 0.1

	joined := strings.Join(terse.Directives(), " ")
	for _, want := range []string{"Say less", "lowercase"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestSpeechStyleDialsRunBothWays(t *testing.T) {
	cold, warm := DefaultSpeechStyle(), DefaultSpeechStyle()
	cold.Warmth, warm.Warmth = 0.1, 0.9

	if strings.Join(cold.Directives(), " ") == strings.Join(warm.Directives(), " ") {
		t.Error("opposite ends of a dial produced the same instruction")
	}
}

func TestParseDialReadsTheCharacterFileSyntax(t *testing.T) {
	style := DefaultSpeechStyle()

	for _, line := range []string{"warmth: 0.8", "  Sarcasm : 0.2 ", "dominance:1"} {
		if !parseDial(line, &style) {
			t.Errorf("failed to read %q", line)
		}
	}
	if style.Warmth != 0.8 || style.Sarcasm != 0.2 || style.Dominance != 1 {
		t.Errorf("parsed %+v", style)
	}
}

// Authored content: a typo in a dial should not stop the bot starting.
func TestParseDialRefusesNonsenseWithoutComplaining(t *testing.T) {
	style := DefaultSpeechStyle()

	for _, line := range []string{
		"warmth: high", "warmth: 1.5", "warmth: -0.2", "sarcasm", "nosuchdial: 0.5", "",
	} {
		if parseDial(line, &style) {
			t.Errorf("accepted %q", line)
		}
	}
	if style != DefaultSpeechStyle() {
		t.Errorf("a rejected line still changed the style: %+v", style)
	}
}

func TestParseCharacterReadsTheTemperSection(t *testing.T) {
	src := `She lives here.

## Temper

- warmth: 0.2
- dominance: 0.9

## Examples

> user: hi
> her: no
`
	c, err := ParseCharacter("X", strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseCharacter: %v", err)
	}
	if c.Style.Warmth != 0.2 || c.Style.Dominance != 0.9 {
		t.Errorf("temper = %+v", c.Style)
	}
	// Untouched dials stay at the middle and stay silent.
	if c.Style.Sarcasm != 0.5 {
		t.Errorf("an unset dial did not default to the middle: %+v", c.Style)
	}
	if strings.Contains(c.Persona, "warmth") {
		t.Errorf("the temper section leaked into the persona:\n%s", c.Persona)
	}
}
