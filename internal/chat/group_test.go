package chat

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// The live log, where she answered a question meant for someone else:
// Duchess asked Big M something, she answered Big M, and Big M's next line —
// "are you on phone?" — was to Duchess.
func seedGroup(h *harness) {
	h.svc.conv.Seed(testChannel, []mind.Turn{
		{UserID: bigM, Username: "Big M", Content: "welcome message is a short template", At: clock},
		{UserID: "u9", Username: "Duchess", Content: "@Big M how do I copy the thread link?", At: clock},
		{FromBot: true, To: bigM, Content: "two templates and a cough - sure, impressive", At: clock},
	})
}

// With others talking, a line right after hers still reaches her, marked as
// not necessarily hers.
func TestAFollowUpInAGroupIsMarkedACrowd(t *testing.T) {
	h := newHarness(t)
	seedGroup(h)
	h.svc.Observe(h.sess, message("m1", "are you on phone?", false))
	tk, ok := h.take()
	if !ok || tk.item.Trigger != mind.TriggerFollowUp || !tk.item.Crowd {
		t.Errorf("queued %+v, %v", tk.item, ok)
	}
}

// Tagging someone else is talking to them, however soon after her.
func TestTaggingSomeoneElseIsNotAFollowUp(t *testing.T) {
	h := newHarness(t)
	seedGroup(h)
	m := message("m1", "@Duchess video guide", false)
	m.Mentions = []*discordgo.User{{ID: "u9", Username: "Duchess"}}
	h.svc.Observe(h.sess, m)
	if tk, ok := h.take(); ok && tk.item.Trigger == mind.TriggerFollowUp {
		t.Errorf("taken as carrying on with her: %+v", tk.item)
	}
}

// Just the two of them, it is still a plain follow-up.
func TestAFollowUpBetweenTwoIsNotACrowd(t *testing.T) {
	h := newHarness(t)
	h.svc.conv.Seed(testChannel, []mind.Turn{
		{UserID: bigM, Username: "Big M", Content: "welcome message is a short template", At: clock},
		{FromBot: true, To: bigM, Content: "two templates and a cough", At: clock},
	})
	h.svc.Observe(h.sess, message("m1", "impressive huh?", false))
	tk, ok := h.take()
	if !ok || tk.item.Trigger != mind.TriggerFollowUp || tk.item.Crowd {
		t.Errorf("queued %+v, %v", tk.item, ok)
	}
}
