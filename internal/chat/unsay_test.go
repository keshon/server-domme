package chat

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
)

const saidID = "s1"

// seedSaid puts a line of hers in the conversation and in her memory, the
// way answering leaves it.
func seedSaid(t *testing.T, h *harness) {
	t.Helper()
	h.svc.conv.Seed(testChannel, []mind.Turn{
		{UserID: bigM, Username: "Big M", Content: "say something", At: clock},
		{FromBot: true, To: bigM, MessageID: saidID, Content: "what she said", At: clock},
	})
	if err := h.memory.AddMoment(testGuild, memory.Moment{
		At: clock, Channel: "chat", Said: saidID, Text: `I said: "what she said"`,
	}); err != nil {
		t.Fatal(err)
	}
}

// deleted reports whether Discord was asked to delete a message.
func (d *discord) deleted(messageID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range d.requests {
		if r.method == http.MethodDelete && strings.HasSuffix(r.path, "/messages/"+messageID) {
			return true
		}
	}
	return false
}

// Taking one of her messages back removes it from the channel, from the
// conversation she is in, and from what she remembers saying.
func TestUnsayTakesTheMessageOutOfEverything(t *testing.T) {
	h := newHarness(t)
	seedSaid(t, h)

	done, err := h.svc.Unsay(h.sess, testGuild, testChannel, saidID)
	if err != nil {
		t.Fatal(err)
	}
	if !h.discord.deleted(saidID) {
		t.Error("Discord was not asked to delete it")
	}
	if !done.FromConversation || done.Forgotten != 1 || done.Text != "what she said" {
		t.Errorf("came to %+v", done)
	}
	for _, turn := range h.svc.conv.Recent(testChannel) {
		if turn.MessageID == saidID {
			t.Error("it is still part of the conversation")
		}
	}
	day, _ := h.memory.Day(testGuild, clock)
	for _, m := range day.Moments {
		if m.Said == saidID {
			t.Error("she still remembers saying it")
		}
	}
}

// Somebody else's message is not hers to take back, and nothing is deleted.
func TestUnsayRefusesAMessageThatIsNotHers(t *testing.T) {
	h := newHarness(t)
	h.discord.author = bigM
	if _, err := h.svc.Unsay(h.sess, testGuild, testChannel, "m1"); !errors.Is(err, ErrNotHers) {
		t.Errorf("err %v", err)
	}
	if h.discord.deleted("m1") {
		t.Error("somebody else's message was deleted")
	}
}

// A message she said before the last restart is gone from the channel and
// from her memory, whether or not the conversation still has it.
func TestUnsayWorksWithNothingInTheConversation(t *testing.T) {
	h := newHarness(t)
	if err := h.memory.AddMoment(testGuild, memory.Moment{
		At: clock, Channel: "chat", Said: saidID, Text: `I said: "what she said"`,
	}); err != nil {
		t.Fatal(err)
	}
	done, err := h.svc.Unsay(h.sess, testGuild, testChannel, saidID)
	if err != nil || done.FromConversation || done.Forgotten != 1 {
		t.Errorf("came to %+v, %v", done, err)
	}
}
