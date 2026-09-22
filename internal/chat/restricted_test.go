package chat

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// restrict marks the test channel age-restricted, and adds an ordinary
// channel and a thread in the restricted one.
func restrict(t *testing.T, h *harness) {
	t.Helper()
	if err := h.sess.State.GuildAdd(&discordgo.Guild{ID: testGuild, Name: "Test", Channels: []*discordgo.Channel{
		{ID: testChannel, GuildID: testGuild, Name: "chat", NSFW: true},
		{ID: "open", GuildID: testGuild, Name: "general"},
		{ID: "thread", GuildID: testGuild, Name: "a thread", ParentID: testChannel, Type: discordgo.ChannelTypeGuildPublicThread},
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestAgeRestrictedIsTheServersMarking(t *testing.T) {
	h := newHarness(t)
	restrict(t, h)
	for channel, want := range map[string]bool{testChannel: true, "thread": true, "open": false, "unknown": false} {
		if got := AgeRestricted(h.sess, channel); got != want {
			t.Errorf("%s: %v, want %v", channel, got, want)
		}
	}
}

// In an age-restricted channel she was let into, nothing is answered and
// nothing is kept.
func TestSheTakesNothingFromAnAgeRestrictedChannel(t *testing.T) {
	h := newHarness(t, appraisal(`"act":"reply","intent":"x"`), "hi")
	restrict(t, h)
	h.svc.Observe(h.sess, message("m1", "@Domme hi", true))
	if _, ok := h.take(); ok {
		t.Error("answered in an age-restricted channel")
	}
	if got := h.svc.conv.Recent(testChannel); len(got) != 0 {
		t.Errorf("kept %v", got)
	}
}

// A channel marked after she was let in to read is not walked through.
func TestAnAgeRestrictedChannelIsNotWalked(t *testing.T) {
	h := newHarness(t)
	h.svc.walks = true
	if err := h.store.SetChatReads(testGuild, "art", true); err != nil {
		t.Fatal(err)
	}
	m := message("m1", "look at this", false)
	m.ChannelID = "art"
	h.svc.Observe(h.sess, m)
	if err := h.sess.State.GuildAdd(&discordgo.Guild{ID: testGuild, Channels: []*discordgo.Channel{{ID: "art", GuildID: testGuild, NSFW: true}}}); err != nil {
		t.Fatal(err)
	}
	if w, _ := h.svc.walk(h.sess, testGuild); w != nil {
		t.Errorf("walked through an age-restricted channel: %+v", w)
	}
}

// Nothing she starts is said in one either.
func TestSheStartsNothingInAnAgeRestrictedChannel(t *testing.T) {
	h := newHarness(t, "the new pins are ugly")
	restrict(t, h)
	if err := h.store.SetChatProactive(testGuild, testChannel, true); err != nil {
		t.Fatal(err)
	}
	h.svc.act(context.Background(), h.sess, testGuild, clock, mind.Idle{GuildID: testGuild, Now: clock},
		mind.Impulse{About: "the new pins", From: "today: the pins changed"})
	if got := h.discord.posted(); len(got) != 0 {
		t.Errorf("spoke in an age-restricted channel: %v", got)
	}
}

// After a restart, what the bot's commands posted is not read back as hers:
// an interaction's response, a follow-up, and a notice replying to one.
func TestCommandOutputIsNotHerWords(t *testing.T) {
	h := newHarness(t)
	self := &discordgo.User{ID: selfUserID, Username: "Domme", Bot: true}
	taskPost := &discordgo.Message{ID: "1", ChannelID: testChannel, Author: self, Content: "**New Task** something explicit",
		Interaction: &discordgo.MessageInteraction{Name: "task"}}
	// Newest first, as Discord returns them.
	history := []*discordgo.Message{
		{ID: "5", ChannelID: testChannel, Author: self, Content: "ok fair"},
		{ID: "4", ChannelID: testChannel, Author: self, Content: "**Task Expired** too slow", ReferencedMessage: taskPost},
		{ID: "3", ChannelID: testChannel, Author: self, Content: "**Task Completed** good", WebhookID: "app"},
		{ID: "2", ChannelID: testChannel, Author: &discordgo.User{ID: bigM, Username: "Big M"}, Content: "done it"},
		taskPost,
	}
	turns := h.svc.historyToTurns(h.sess, history)
	if len(turns) != 2 || turns[0].Content != "done it" || turns[1].Content != "ok fair" || !turns[1].FromBot {
		t.Errorf("turns %+v", turns)
	}
}
