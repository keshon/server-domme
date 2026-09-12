package chat

import (
	"strings"
	"testing"
)

// Being switched off and failing to start produce the same silence in a
// channel, and the message is the only thing that tells an administrator which
// one they are looking at.
func TestUnavailableMessageDistinguishesOffFromBroken(t *testing.T) {
	off := unavailableMessage("")
	if !strings.Contains(off, "CHAT_ENABLED") {
		t.Errorf("the switched-off message does not name the setting to change:\n%s", off)
	}

	broken := unavailableMessage("the character file could not be read")
	if strings.Contains(broken, "CHAT_ENABLED") {
		t.Errorf("a start-up failure was reported as the feature being switched off:\n%s", broken)
	}
	if !strings.Contains(broken, "the character file could not be read") {
		t.Errorf("the reason was dropped:\n%s", broken)
	}
}

// The reason is the whole payload. Reporting that something went wrong without
// saying what is what sent an operator to check a setting they had already set.
func TestUnavailableMessageCarriesTheReasonVerbatim(t *testing.T) {
	reason := "no chat backend could be reached at startup"
	got := unavailableMessage(reason)

	if !strings.HasSuffix(strings.TrimSpace(got), reason) {
		t.Errorf("the reason is not carried through intact:\n%s", got)
	}
}

func TestTrimForEmbedFlattensAndCuts(t *testing.T) {
	got := trimForEmbed("line one\n  line two\ttabbed")
	if got != "line one line two tabbed" {
		t.Errorf("trimForEmbed = %q, want it flattened onto one line", got)
	}

	// A relay can answer with a whole HTML error page from a proxy in front of
	// it, which Discord would refuse as an embed.
	long := trimForEmbed(strings.Repeat("x", maxBackendErrorChars*3))
	if len(long) > maxBackendErrorChars+len("…") {
		t.Errorf("trimForEmbed left %d chars, want it cut to %d", len(long), maxBackendErrorChars)
	}
}

func TestMeterDrawsTheRangeAndClampsOutsideIt(t *testing.T) {
	empty := meter(0)
	half := meter(0.5)
	full := meter(1)

	if strings.Count(empty, "█") != 0 {
		t.Errorf("meter(0) = %s, want no blocks", empty)
	}
	if strings.Count(full, "█") != meterWidth {
		t.Errorf("meter(1) = %s, want a full bar", full)
	}
	if n := strings.Count(half, "█"); n != meterWidth/2 {
		t.Errorf("meter(0.5) drew %d of %d blocks", n, meterWidth)
	}

	// A drive should never be outside 0..1, but a bar that panics on one
	// would take the command down with it.
	for _, v := range []float64{-5, 5} {
		if got := strings.Count(meter(v), "█"); got < 0 || got > meterWidth {
			t.Errorf("meter(%v) drew %d blocks", v, got)
		}
	}
}
