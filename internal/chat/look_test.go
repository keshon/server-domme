package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// ready puts a room she reads in front of her, with something said in it.
func ready(t *testing.T, h *harness) {
	t.Helper()
	if err := h.sess.State.GuildAdd(&discordgo.Guild{ID: testGuild, Channels: []*discordgo.Channel{
		{ID: testChannel, GuildID: testGuild, Name: "chat"},
		{ID: "r1", GuildID: testGuild, Name: "roundtable"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetChatReads(testGuild, "r1", true); err != nil {
		t.Fatal(err)
	}
	h.svc.walks, h.svc.looks = true, true
	h.svc.conv.Seed("r1", []mind.Turn{
		{UserID: "u9", Username: "Duchess", Content: "so are we approving the pics or not", At: clock.Add(-time.Minute)},
	})
}

func scene(h *harness) mind.Scene {
	return mind.Scene{GuildID: testGuild, ChannelID: testChannel, ChannelName: "chat", Now: clock,
		UserID: bigM, Username: "Big M"}
}

// Asked what is happening in a room she reads, she goes and looks, and what
// she saw is written down as a walk she took herself.
func TestSheGoesAndLooksWhenSheAsks(t *testing.T) {
	h := newHarness(t, `{"caught":"they are still arguing about approving pics","weight":0.2}`)
	ready(t, h)
	caught, why := h.svc.lookAt(context.Background(), h.sess, scene(h), "roundtable")
	if why != "" || caught == "" {
		t.Fatalf("caught %q, held back: %q", caught, why)
	}
	day, _ := h.memory.Day(testGuild, clock)
	if len(day.Moments) != 1 || !day.Moments[0].Walk || !strings.Contains(day.Moments[0].Text, "went to look at #roundtable") {
		t.Errorf("wrote %+v", day.Moments)
	}
	if !strings.Contains(h.provider.sent[0][1].Content, "approving the pics") {
		t.Error("the room's lines were not read")
	}
	// Just been: not again.
	if _, why := h.svc.lookAt(context.Background(), h.sess, scene(h), "roundtable"); why == "" {
		t.Error("she went back in straight away")
	}
}

// The rails: only a room she reads, only while looking is on, and only so
// often in a day.
func TestTheRailsOnLooking(t *testing.T) {
	h := newHarness(t)
	ready(t, h)
	if _, why := h.svc.lookAt(context.Background(), h.sess, scene(h), "ticket-mega"); why == "" {
		t.Error("she looked into a room she does not read")
	}
	h.svc.looks = false
	if _, why := h.svc.lookAt(context.Background(), h.sess, scene(h), "roundtable"); why == "" {
		t.Error("she looked with looking switched off")
	}
	h.svc.looks = true
	for i := 0; i < looksPerDay; i++ {
		h.svc.looked(testGuild, clock)
	}
	if h.svc.mayLook(testGuild, clock) {
		t.Error("the day's budget did not run out")
	}
	if h.svc.mayLook(testGuild, clock.AddDate(0, 0, 1)) == false {
		t.Error("the budget did not start again the next day")
	}
}

// The rooms she reads are named in her thinking, so she can ask for one.
func TestTheRoomsSheReadsAreNamed(t *testing.T) {
	h := newHarness(t)
	ready(t, h)
	if got := h.svc.readNames(h.sess, testGuild); len(got) != 1 || got[0] != "roundtable" {
		t.Errorf("rooms %v", got)
	}
}
