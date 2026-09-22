package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
)

// Step 3 of docs/persona-v3.md: follow-ups on sight, joining out of
// interest, and whether what she started was welcome.

// Someone she meant to follow up with turns up and says something not aimed
// at her: the intention comes due now, as a moment of its own.
func TestAFollowUpComesDueWhenThePersonShowsUp(t *testing.T) {
	h := newHarness(t,
		appraisal(`"act":"reply","intent":"ask how the naming went"`),
		"so, did cityscope stick?",
	)
	h.svc.followUpOnSight = true
	if err := h.memory.AddThread(testGuild, memory.Thread{
		Due: clock.Add(-time.Hour), Person: memory.Ref{ID: bigM, Name: "Big M"}, Text: "ask what he named it",
	}); err != nil {
		t.Fatal(err)
	}

	h.svc.Observe(h.sess, message("m1", "back from lunch, what did I miss", false))
	tk, ok := h.take()
	if !ok {
		t.Fatal("nothing queued when he turned up")
	}
	if tk.item.Trigger != mind.TriggerSight || tk.item.Thread == nil {
		t.Fatalf("queued %s with thread %v", tk.item.Trigger, tk.item.Thread)
	}
	h.svc.handle(context.Background(), tk)

	if got := h.discord.posted(); len(got) != 1 {
		t.Fatalf("posted %v", got)
	}
	prompt := h.provider.sent[0][1].Content
	if !strings.Contains(prompt, "ask what he named it") {
		t.Errorf("the appraisal did not see what she meant to do:\n%s", prompt)
	}
	threads, _ := h.memory.Threads(testGuild)
	if len(memory.Unfinished(threads)) != 0 {
		t.Errorf("the follow-up is still open: %+v", threads)
	}
	if len(h.svc.startedMsgs) != 1 {
		t.Errorf("the follow-up is not being watched for how it lands")
	}

	// The same person talking on does not bring it up again.
	h.svc.Observe(h.sess, message("m2", "anyway, lunch was good", false))
	if tk, ok := h.take(); ok && tk.item.Trigger == mind.TriggerSight {
		t.Error("brought up twice")
	}
}

func TestNoFollowUpOnSightWhenSwitchedOff(t *testing.T) {
	h := newHarness(t)
	if err := h.memory.AddThread(testGuild, memory.Thread{Person: memory.Ref{ID: bigM, Name: "Big M"}, Text: "ask about it"}); err != nil {
		t.Fatal(err)
	}
	h.svc.Observe(h.sess, message("m1", "hello all of you", false))
	if _, ok := h.take(); ok {
		t.Error("followed up with the switch off")
	}
}

// She notices an overheard remark because it touches something of hers,
// not because of a roll of the dice.
func TestAnOverheardRemarkGetsHerAttentionByWhatItTouches(t *testing.T) {
	h := newHarness(t)
	h.svc.interest = true
	h.svc.roll = func() float64 { return 0.99 }
	h.svc.character.Specifics = []string{"hates Rust because the compiler lectures her"}
	if err := h.store.SetChatProactive(testGuild, testChannel, true); err != nil {
		t.Fatal(err)
	}

	other := func(id, text string) *discordgo.MessageCreate {
		m := message(id, text, false)
		m.Author = &discordgo.User{ID: "u9", Username: "Stranger"}
		return m
	}
	h.svc.Observe(h.sess, other("m1", "what a lovely afternoon for a walk outside"))
	if _, ok := h.take(); ok {
		t.Error("noticed a remark that touches nothing of hers")
	}
	h.svc.Observe(h.sess, other("m2", "honestly the rust compiler is so pedantic today"))
	tk, ok := h.take()
	if !ok || tk.item.Trigger != mind.TriggerOverheard {
		t.Fatal("did not notice a remark about something she has opinions on")
	}
}

// What she started is watched: answered by the person it was aimed at, and
// the outcome lands in her memory as something that happened.
func TestWhatSheStartedIsAnsweredOrIgnored(t *testing.T) {
	h := newHarness(t)
	sc := mind.Scene{GuildID: testGuild, ChannelID: testChannel, ChannelName: "chat", UserID: bigM, Username: "Big M"}
	h.svc.watchStarted(sc, formReach, "s1", "how did the interview go?")
	h.svc.watchStarted(mind.Scene{GuildID: testGuild, ChannelID: "c2", ChannelName: "quiet"}, formStart, "s2", "anyone around?")

	clock = clock.Add(3 * time.Minute)
	defer func() { clock = clock.Add(-3*time.Minute - welcomeWindow) }()
	h.svc.Observe(h.sess, message("m1", "it went fine thanks", false))

	clock = clock.Add(welcomeWindow)
	h.svc.sweepStarted(clock)

	day, _ := h.memory.Day(testGuild, clock)
	var got []string
	for _, mo := range day.Moments {
		got = append(got, mo.Text)
	}
	all := strings.Join(got, "\n")
	if !strings.Contains(all, "Big M answered") {
		t.Errorf("the answer was not remembered:\n%s", all)
	}
	if !strings.Contains(all, "nobody answered") {
		t.Errorf("the silence was not remembered:\n%s", all)
	}
	if len(h.svc.startedMsgs) != 0 {
		t.Errorf("still watching %d", len(h.svc.startedMsgs))
	}
}

func TestAReactionCountsWhenNobodyAnswers(t *testing.T) {
	h := newHarness(t)
	sc := mind.Scene{GuildID: testGuild, ChannelID: testChannel, UserID: bigM, Username: "Big M"}
	h.svc.watchStarted(sc, formSight, "s1", "so?")
	h.svc.ObserveReaction(&discordgo.MessageReaction{MessageID: "s1", UserID: "u9"})
	h.svc.ObserveReaction(&discordgo.MessageReaction{MessageID: "s1", UserID: bigM})
	h.svc.sweepStarted(clock.Add(welcomeWindow))
	day, _ := h.memory.Day(testGuild, clock.Add(welcomeWindow))
	if len(day.Moments) != 1 || !strings.Contains(day.Moments[0].Text, "got a reaction") {
		t.Errorf("moments %+v", day.Moments)
	}
}

// The base rate: how many messages in a room got an answer from anyone.
func TestRoomsCountHowOftenAnyoneIsAnswered(t *testing.T) {
	h := newHarness(t)
	at := clock
	for i, who := range []string{"a", "a", "b", "c", "c"} {
		h.svc.noteRoom(testGuild, testChannel, who, at.Add(time.Duration(i)*time.Minute))
	}
	rates := h.svc.roomRates(testGuild, clock)
	if len(rates) != 1 {
		t.Fatalf("rates %+v", rates)
	}
	// a→a no, a→b yes, b→c yes, c→c no: 2 of 5 answered, 3 unanswered.
	if rates[0].Messages != 5 || rates[0].Unanswered != 3 {
		t.Errorf("rate %+v", rates[0])
	}
}
