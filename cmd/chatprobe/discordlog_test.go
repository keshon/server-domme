package main

import (
	"strings"
	"testing"
	"time"
)

// The shape Discord copies a conversation in, taken from the production log:
// her name and the APP badge on lines of their own, a full date for old
// messages, "Yesterday at" and a bare time for recent ones.
const copied = `Big M — 12.09.2026 14:06
@Server Domme Welcome to your new home
Server Domme
APP
 — 12.09.2026 14:06
this is not my new home. i just renewed the lease on being here
Big M — Yesterday at 23:39
Explain this to me
I'm waiting
Server Domme
APP
 — 15:52
@Big M hey, you still there?
`

func TestParseLogReadsDiscordsCopiedShape(t *testing.T) {
	today := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	msgs, err := parseLog(strings.NewReader(copied), "Server Domme", today, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	want := []message{
		{Author: "Big M", At: time.Date(2026, 9, 12, 14, 6, 0, 0, time.UTC), Text: "@Server Domme Welcome to your new home"},
		{Author: "Server Domme", At: time.Date(2026, 9, 12, 14, 6, 0, 0, time.UTC), Text: "this is not my new home. i just renewed the lease on being here", Bot: true},
		{Author: "Big M", At: time.Date(2026, 9, 20, 23, 39, 0, 0, time.UTC), Text: "Explain this to me"},
		{Author: "Big M", At: time.Date(2026, 9, 20, 23, 39, 0, 0, time.UTC), Text: "I'm waiting"},
		{Author: "Server Domme", At: time.Date(2026, 9, 21, 15, 52, 0, 0, time.UTC), Text: "@Big M hey, you still there?", Bot: true},
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d messages, want %d: %+v", len(msgs), len(want), msgs)
	}
	for i := range want {
		if msgs[i] != want[i] {
			t.Errorf("message %d is %+v, want %+v", i, msgs[i], want[i])
		}
	}
}
