package chat

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Discord refuses a message over 2000 characters whole; a long reply goes
// out as several instead of not at all, with nothing lost between them.
func TestPartsKeepEveryMessageUnderDiscordsLimit(t *testing.T) {
	line := strings.Repeat("word ", 30) + "\n"
	long := strings.Repeat(line, 40) // about 6000 characters, one paragraph
	got := parts(long)
	if len(got) < 3 {
		t.Fatalf("got %d messages, want the reply split", len(got))
	}
	for i, p := range got {
		if n := utf8.RuneCountInString(p); n > maxMessage {
			t.Errorf("message %d is %d characters", i, n)
		}
	}
	if strings.Join(strings.Fields(strings.Join(got, " ")), " ") != strings.Join(strings.Fields(long), " ") {
		t.Error("words were lost or changed in the split")
	}
}

// A code block cut in two is closed and opened again, so neither message
// leaves one open.
func TestPartsCloseACodeBlockTheyCut(t *testing.T) {
	long := "look:\n```\n" + strings.Repeat("some code here\n", 200) + "```"
	got := parts(long)
	if len(got) < 2 {
		t.Fatalf("got %d messages, want the block split", len(got))
	}
	for i, p := range got {
		if strings.Count(p, "```")%2 != 0 {
			t.Errorf("message %d leaves a block open:\n%s", i, p)
		}
		if utf8.RuneCountInString(p) > maxMessage {
			t.Errorf("message %d is too long", i)
		}
	}
}

func TestPartsLeaveAShortReplyAlone(t *testing.T) {
	if got := parts("fine.\n\nnot a wave off"); len(got) != 2 || got[0] != "fine." {
		t.Errorf("parts = %q", got)
	}
	if got := parts("```\nx\n```"); len(got) != 1 {
		t.Errorf("a short code reply was split: %q", got)
	}
}
