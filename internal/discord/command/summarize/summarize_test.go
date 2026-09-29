package summarize

import (
	"testing"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

func TestBuildTranscriptSkipsBotsAndEmpty(t *testing.T) {
	msgs := []adapter.ListedMessage{
		{AuthorName: "a", Content: "hello"},
		{AuthorName: "b", Content: "", Bot: false},
		{AuthorName: "bot", Content: "beep", Bot: true},
		{AuthorName: "c", Content: "  world  "},
	}
	text, count := BuildTranscript(msgs)
	if count != 2 {
		t.Fatalf("count=%d text=%q", count, text)
	}
	if got := "c: world\na: hello"; text != "a: hello\nc: world" && text != got {
		// order is oldest-first: input is newest-first so last element first.
		t.Fatalf("text=%q", text)
	}
}

func TestBuildTranscriptEmpty(t *testing.T) {
	if _, count := BuildTranscript(nil); count != 0 {
		t.Fatalf("count=%d", count)
	}
}
