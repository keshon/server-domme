package ai

import (
	"strings"
	"testing"
)

func TestCleanStripsReasoningBlocks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"closed think", "<think>weighing it up</think>Fine, you win.", "Fine, you win."},
		{"tag variants", "<reasoning>hm</reasoning>ok", "ok"},
		{"unclosed think", "Sure.\n<think>still going when the tokens ran", "Sure."},
		{"no block", "just words", "just words"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Clean(tc.in); got != tc.want {
				t.Errorf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCleanStripsCopiedSpeakerLabel(t *testing.T) {
	// The conversation is handed over as "username: text", and models imitate
	// the format in their own output.
	if got := Clean("Domme: get back to work"); got != "get back to work" {
		t.Errorf("Clean() = %q, want the label stripped", got)
	}
}

func TestCleanKeepsColonsThatAreNotLabels(t *testing.T) {
	// A sentence may legitimately open with a colon-bearing clause; only a
	// short bare token before the colon is a speaker label.
	in := "here is the thing: you were late"
	if got := Clean(in); got != in {
		t.Errorf("Clean(%q) = %q, want unchanged", in, got)
	}
}

func TestCleanStripsOnlyMatchedWrappingQuotes(t *testing.T) {
	if got := Clean(`"as if that would work"`); got != "as if that would work" {
		t.Errorf("wrapping quotes not stripped: %q", got)
	}
	// Stripping here would weld two quoted fragments into one.
	in := `"first" and "second"`
	if got := Clean(in); got != in {
		t.Errorf("Clean(%q) = %q, want unchanged", in, got)
	}
}

func TestCleanTruncatesToDiscordLimit(t *testing.T) {
	long := strings.Repeat("word ", 900)
	got := Clean(long)
	if len([]rune(got)) > MaxReplyChars {
		t.Fatalf("reply is %d runes, over the %d limit Discord enforces",
			len([]rune(got)), MaxReplyChars)
	}
}

func TestCleanPrefersSentenceBoundaryWhenTruncating(t *testing.T) {
	body := strings.Repeat("a sentence that keeps going. ", 100)
	got := Clean(body)
	if !strings.HasSuffix(got, ".") {
		t.Errorf("truncation did not fall back to a sentence boundary: ends %q",
			got[max(0, len(got)-40):])
	}
}

// Observed from a live relay: the reply ends, then a blank line, then a real
// member's name and a line put in their mouth. Posting that verbatim is the
// worst thing this bot can do to a channel.
func TestCleanCutsAContinuedConversation(t *testing.T) {
	raw := "quiet isn't gone. i've been here.\n\nnewbie: usually. you get used to it, or you don't."
	got := Clean(raw)
	if strings.Contains(got, "newbie:") {
		t.Errorf("Clean left another speaker's line in: %q", got)
	}
	if got != "quiet isn't gone. i've been here." {
		t.Errorf("Clean = %q, want just her own line", got)
	}
}

func TestCleanCutsAContinuedConversationWithTimestamps(t *testing.T) {
	raw := "fine.\n\ncass (5 minutes ago): actually no"
	if got := Clean(raw); strings.Contains(got, "cass") {
		t.Errorf("Clean left a timestamped speaker line in: %q", got)
	}
}

func TestCleanKeepsAnOrdinaryMultiLineReply(t *testing.T) {
	raw := "read the pins.\nthen ask again if it still does not make sense."
	if got := Clean(raw); got != raw {
		t.Errorf("Clean = %q, want the reply unchanged", got)
	}
}

// A reply that is nothing but a labelled line has its label stripped by
// speakerPrefix; cutting there as well would leave an empty message.
func TestCleanDoesNotEmptyASelfLabelledReply(t *testing.T) {
	if got := Clean("Domme: still here"); got != "still here" {
		t.Errorf("Clean = %q, want %q", got, "still here")
	}
}
