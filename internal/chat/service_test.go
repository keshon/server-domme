package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

const (
	selfUserID  = "bot-1"
	testGuild   = "g1"
	testChannel = "c1"
	bigM        = "u1"
)

// clock is a fixed afternoon, inside the hours she may start things.
var clock = time.Date(2026, 9, 19, 14, 0, 0, 0, time.UTC)

// scripted answers each call with the next reply in line.
type scripted struct {
	mu      sync.Mutex
	replies []string
	sent    [][]ai.Message
}

func (p *scripted) Generate(_ context.Context, msgs []ai.Message) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, msgs)
	if len(p.replies) == 0 {
		return "", ai.ErrNoBackend
	}
	r := p.replies[0]
	p.replies = p.replies[1:]
	return r, nil
}

// discord records what the service asked Discord to do, and answers every
// request as a success. It stands in for the network, not for discordgo: the
// requests are the real ones the library builds.
type discord struct {
	mu       sync.Mutex
	requests []request
}

type request struct {
	method, path string
	body         map[string]any
}

func (d *discord) RoundTrip(r *http.Request) (*http.Response, error) {
	req := request{method: r.Method, path: r.URL.Path}
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &req.body)
	}
	d.mu.Lock()
	d.requests = append(d.requests, req)
	d.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"sent-1","channel_id":"c1"}`)),
		Request:    r,
	}, nil
}

// posted is the content of every message the service sent.
func (d *discord) posted() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, r := range d.requests {
		if r.method == http.MethodPost && strings.HasSuffix(r.path, "/messages") {
			content, _ := r.body["content"].(string)
			out = append(out, content)
		}
	}
	return out
}

func (d *discord) reacted() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range d.requests {
		if r.method == http.MethodPut && strings.Contains(r.path, "/reactions/") {
			return true
		}
	}
	return false
}

type harness struct {
	svc      *Service
	store    *storage.Storage
	memory   *memory.Store
	provider *scripted
	discord  *discord
	sess     *discordgo.Session
}

func newHarness(t *testing.T, replies ...string) *harness {
	t.Helper()
	store, err := storage.NewStorage(t.TempDir(), zerolog.Nop())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatal(err)
	}
	mem, err := memory.Open(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}

	d := &discord{}
	state := discordgo.NewState()
	state.User = &discordgo.User{ID: selfUserID, Username: "Domme"}
	sess := &discordgo.Session{State: state, Client: &http.Client{Transport: d}, Ratelimiter: discordgo.NewRatelimiter()}

	p := &scripted{replies: replies}
	svc := New(Deps{
		Character: &mind.Character{Name: "Domme", Persona: "a fixture of this server"},
		Provider:  p,
		Storage:   store,
		Memory:    mem,
		Session:   func() *discordgo.Session { return sess },
		Log:       zerolog.Nop(),
		Roll:      func() float64 { return 0 },
		Now:       func() time.Time { return clock },
	})
	// The tests drive a fixed clock; settling and typing are their own
	// concern, and nothing here sleeps.
	svc.settleQuiet = 0
	svc.sleep = func(context.Context, time.Duration) bool { return true }
	return &harness{svc: svc, store: store, memory: mem, provider: p, discord: d, sess: sess}
}

func message(id, content string, mentionsBot bool) *discordgo.MessageCreate {
	m := &discordgo.MessageCreate{Message: &discordgo.Message{
		ID: id, GuildID: testGuild, ChannelID: testChannel, Content: content,
		Author: &discordgo.User{ID: bigM, Username: "Big M"},
	}}
	if mentionsBot {
		m.Mentions = []*discordgo.User{{ID: selfUserID}}
	}
	return m
}

// take returns the moment the service queued, if any.
func (h *harness) take() (task, bool) {
	select {
	case t := <-h.svc.work:
		return t, true
	default:
		return task{}, false
	}
}

// say has Big M say something and runs whatever it queued through a
// worker, the way the running service does.
func (h *harness) say(t *testing.T, id, content string, mentionsBot bool) {
	t.Helper()
	h.svc.Observe(h.sess, message(id, content, mentionsBot))
	if tk, ok := h.take(); ok {
		h.svc.handle(context.Background(), tk)
		h.svc.doneAnswering(answerKey(tk.item.GuildID, tk.item.ChannelID, tk.item.UserID))
	}
}

func appraisal(fields string) string { return "{" + fields + "}" }

func TestObserveOnlyListensWhereSheWasLetIn(t *testing.T) {
	h := newHarness(t)
	m := message("m1", "@Domme hello", true)
	m.ChannelID = "elsewhere"
	h.svc.Observe(h.sess, m)
	if _, ok := h.take(); ok {
		t.Error("answered in a channel nobody opted in")
	}

	bot := message("m2", "@Domme hello", true)
	bot.Author.Bot = true
	h.svc.Observe(h.sess, bot)
	if _, ok := h.take(); ok {
		t.Error("answered another bot")
	}
}

func TestAMentionIsAnsweredAndRememberedWithWhatSheMeant(t *testing.T) {
	h := newHarness(t,
		appraisal(`"read":"he wants my opinion on his project","feel":"curious","toward":"warming up","mood":"awake","act":"reply","intent":"say the code city idea is clever","note":"building a code-city visualiser"`),
		"that's clever, actually.",
	)
	h.say(t, "m1", "@Domme I made an app that shows code as a city", true)

	posted := h.discord.posted()
	if len(posted) != 1 || posted[0] != "that's clever, actually" {
		t.Fatalf("posted %q", posted)
	}

	day, _ := h.memory.Day(testGuild, clock)
	if len(day.Moments) == 0 || !strings.Contains(day.Moments[len(day.Moments)-1].Text, "meant: say the code city idea is clever") {
		t.Errorf("what she said was not remembered with what she meant: %+v", day.Moments)
	}
	p, ok, _ := h.memory.Person(testGuild, bigM)
	if !ok || p.Feeling != "warming up" || len(p.Notes) != 1 {
		t.Errorf("dossier is %+v", p)
	}
	entries := h.store.MindJournalIn(testGuild, testChannel)
	if len(entries) != 1 || entries[0].Outcome != outcomeAnswered || entries[0].Read == "" {
		t.Errorf("journal is %+v", entries)
	}
}

// A silence looks exactly like a broken bot. She may choose one; the second in
// a row from the same person is overruled.
func TestSheNeverIgnoresTheSamePersonTwiceRunning(t *testing.T) {
	h := newHarness(t,
		appraisal(`"read":"pinging again","act":"ignore"`),
		appraisal(`"read":"still pinging","act":"ignore"`),
		"what.",
	)
	h.say(t, "m1", "@Domme are you bored?", true)
	if got := h.discord.posted(); len(got) != 0 {
		t.Fatalf("the first silence was overruled: %q", got)
	}
	h.say(t, "m2", "@Domme yeah keep ignoring me", true)
	if got := h.discord.posted(); len(got) != 1 {
		t.Fatalf("the second silence in a row stood: %q", got)
	}
}

// The production log: v1 posted "SKIP" to the channel.
func TestAControlWordNeverReachesTheChannel(t *testing.T) {
	h := newHarness(t, appraisal(`"act":"reply","intent":"explain"`), "SKIP")
	h.say(t, "m1", "@Domme explain this to me", true)
	if got := h.discord.posted(); len(got) != 0 {
		t.Fatalf("posted %q", got)
	}
	// An answer is owed, so it is held for another try rather than lost —
	// with its appraisal, so the retry does not think it through twice.
	due := h.svc.deferrals.Due(clock.Add(mind.DeferralRetry))
	if len(due) != 1 || due[0].Considered == nil || due[0].Considered.Intent != "explain" {
		t.Fatalf("held %+v", due)
	}
}

func TestAReactionIsAReactionAndNotAMessage(t *testing.T) {
	h := newHarness(t, appraisal(`"act":"react","emoji":"🫡"`))
	h.say(t, "m1", "@Domme yes ma'am", true)
	if !h.discord.reacted() || len(h.discord.posted()) != 0 {
		t.Fatalf("requests: %+v", h.discord.requests)
	}
}

func TestAnUnreadableAppraisalStillAnswersADirectQuestion(t *testing.T) {
	h := newHarness(t, "she would probably just say hi", "hi.")
	h.say(t, "m1", "@Domme hi", true)
	if got := h.discord.posted(); len(got) != 1 {
		t.Fatalf("posted %q", got)
	}
}

func TestBeingToldToBackOffWithdrawsConsent(t *testing.T) {
	h := newHarness(t, appraisal(`"act":"reply","intent":"fine","back_off":true`), "fine.")
	if err := h.store.SetMindConsent(testGuild, bigM, ConsentOn, clock); err != nil {
		t.Fatal(err)
	}
	h.say(t, "m1", "@Domme stop pinging me please", true)
	if p := h.store.GetMindPerson(testGuild, bigM); p == nil || Consented(p.Attention) {
		t.Fatalf("consent survived: %+v", p)
	}
}

func TestAFollowUpNeedsHerToBeTalkingToThem(t *testing.T) {
	h := newHarness(t, appraisal(`"act":"reply","intent":"hello"`), "hey.")
	h.say(t, "m1", "@Domme hey", true)

	h.svc.Observe(h.sess, message("m2", "how are you", false))
	tk, ok := h.take()
	if !ok || tk.item.Trigger != mind.TriggerFollowUp {
		t.Fatalf("the next line from him was not a follow-up: %+v %v", tk.item, ok)
	}

	other := message("m3", "unrelated chatter", false)
	other.Author = &discordgo.User{ID: "u2", Username: "Big N"}
	h.svc.Observe(h.sess, other)
	if _, ok := h.take(); ok {
		t.Error("someone else's line was taken as meant for her")
	}
}

func TestSheGoesToSomeoneForAReasonAndRemembersIt(t *testing.T) {
	h := newHarness(t,
		`{"choice": 1, "why": "I want to know what he named it", "intent": "ask whether code-scrap got a real name"}`,
		"@Big M did code-scrap ever get a real name",
	)
	if err := h.store.SetMindConsent(testGuild, bigM, ConsentOn, clock); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.SeeMindPerson(testGuild, bigM, "Big M", clock.Add(-30*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := h.store.ExchangeMindPerson(testGuild, bigM, testChannel, clock.Add(-30*time.Hour)); err != nil {
		t.Fatal(err)
	}
	thread := memory.Thread{Due: clock.Add(-time.Hour), Person: memory.Ref{ID: bigM, Name: "Big M"}, Text: "ask what he named his project"}
	if err := h.memory.AddThread(testGuild, thread); err != nil {
		t.Fatal(err)
	}
	h.svc.conv.Seed(testChannel, nil)

	h.svc.considerStarting(context.Background(), testGuild, []string{testChannel})

	posted := h.discord.posted()
	if len(posted) != 1 || !strings.HasPrefix(posted[0], "<@"+bigM+">") {
		t.Fatalf("posted %q", posted)
	}
	day, _ := h.memory.Day(testGuild, clock)
	last := day.Moments[len(day.Moments)-1].Text
	if !strings.Contains(last, "because I want to know what he named it") {
		t.Errorf("her reason was not remembered: %q", last)
	}
	threads, _ := h.memory.Threads(testGuild)
	if len(memory.Unfinished(threads)) != 0 {
		t.Errorf("the thread she acted on is still open: %+v", threads)
	}
	if p := h.store.GetMindPerson(testGuild, bigM); p.Unanswered != 1 {
		t.Errorf("the reach was not counted: %+v", p)
	}
}

func TestSheStartsNothingAtNight(t *testing.T) {
	h := newHarness(t, `{"choice": 1, "why": "x", "intent": "y"}`, "hi")
	night := time.Date(2026, 9, 19, 2, 0, 0, 0, time.UTC)
	h.svc.now = func() time.Time { return night }
	if err := h.store.SetChatProactive(testGuild, testChannel, true); err != nil {
		t.Fatal(err)
	}
	h.svc.conv.Seed(testChannel, nil)
	h.svc.considerStarting(context.Background(), testGuild, []string{testChannel})
	if len(h.provider.sent) != 0 || len(h.discord.posted()) != 0 {
		t.Fatal("she considered starting something at two in the morning")
	}
}

func TestSheReflectsOnYesterdayOnce(t *testing.T) {
	h := newHarness(t, `{"summary":"a quiet day","lately":"quiet","people":[],"done":[],"threads":[]}`)
	h.svc.reflectHour = 5
	yesterday := clock.AddDate(0, 0, -1)
	if err := h.memory.AddMoment(testGuild, memory.Moment{At: yesterday, Text: "said hi"}); err != nil {
		t.Fatal(err)
	}
	h.svc.reflectDue(context.Background())
	h.svc.reflectDue(context.Background())
	day, _ := h.memory.Day(testGuild, yesterday)
	if day.Summary != "a quiet day" || len(h.provider.sent) != 1 {
		t.Fatalf("summary %q after %d calls", day.Summary, len(h.provider.sent))
	}
}

// A Discord id is a stable identifier for a real account, and the model has
// no business seeing one. Everything a message can carry one in is rewritten.
func TestIDsNeverReachTheModel(t *testing.T) {
	h := newHarness(t, appraisal(`"act":"ignore"`))
	if err := h.sess.State.GuildAdd(&discordgo.Guild{ID: testGuild, Name: "Test",
		Channels: []*discordgo.Channel{{ID: testChannel, GuildID: testGuild, Name: "chat"}, {ID: "148773639602962435", GuildID: testGuild, Name: "general"}},
		Roles:    []*discordgo.Role{{ID: "248773639602962435", Name: "sub"}},
	}); err != nil {
		t.Fatal(err)
	}
	m := message("m1", "@Domme <@365177820663513089> said hi in <#148773639602962435> to <@&248773639602962435> <:salutt:998877665544332211> "+
		"see https://discord.com/channels/1/2/3 and his id is 365177820663513089", true)
	m.Mentions = append(m.Mentions, &discordgo.User{ID: "365177820663513089", Username: "Big N"})
	h.svc.Observe(h.sess, m)
	tk, ok := h.take()
	if !ok {
		t.Fatal("nothing queued")
	}
	h.svc.handle(context.Background(), tk)

	id := regexp.MustCompile(`\d{17,20}`)
	for _, msgs := range h.provider.sent {
		for _, msg := range msgs {
			if found := id.FindString(msg.Content); found != "" {
				t.Fatalf("an id reached the model: %s in\n%s", found, msg.Content)
			}
		}
	}
	prompt := h.provider.sent[0][1].Content
	for _, want := range []string{"@Big N", "#general", "@sub", ":salutt:"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("%q missing from what she read:\n%s", want, prompt)
		}
	}
}

// From production: Big M tagged Big N to introduce her, and "Big N, welcome"
// reached the channel as plain text.
func TestSheCanTagWhoeverTheyTagged(t *testing.T) {
	h := newHarness(t)
	sc := mind.Scene{
		GuildID: testGuild, ChannelID: testChannel, Trigger: mind.TriggerReply,
		UserID: bigM, Username: "Big M", MessageID: "m1",
		Turns: []mind.Turn{{UserID: bigM, Username: "Big M", MessageID: "m1",
			Content: "fiiine... @Big N meet @Domme", Tagged: []mind.Person{{ID: "u2", Name: "Big N"}}}},
	}
	msg := h.svc.outgoing(sc, "Big N, welcome. Big M, you did the thing.")
	if msg.Content != "<@u2>, welcome. <@u1>, you did the thing." {
		t.Errorf("content %q", msg.Content)
	}
	if got := msg.AllowedMentions.Users; len(got) != 2 {
		t.Errorf("may notify %v", got)
	}

	// Someone nobody tagged stays unnotified, however she names them.
	sc.Turns = append(sc.Turns, mind.Turn{UserID: "u3", Username: "cass", Content: "hi"})
	msg = h.svc.outgoing(sc, "@cass, you too")
	for _, id := range msg.AllowedMentions.Users {
		if id == "u3" {
			t.Errorf("she could notify someone nobody tagged: %v", msg.AllowedMentions.Users)
		}
	}
}

// A second thought is born with the reply and sent once its moment comes —
// as its own message, not anchored, and remembered as hers.
func TestASecondThoughtFollowsHerReply(t *testing.T) {
	h := newHarness(t,
		appraisal(`"act":"reply","intent":"say it is clever","then":"ask what the ugly neighbourhoods look like","then_after":40`),
		"that's clever, actually",
		"what do the ugly neighbourhoods look like",
	)
	h.say(t, "m1", "@Domme I made an app that shows code as a city", true)

	h.svc.dueThoughts(context.Background())
	if got := h.discord.posted(); len(got) != 1 {
		t.Fatalf("the thought went out before its moment: %q", got)
	}
	h.svc.now = func() time.Time { return clock.Add(time.Minute) }
	h.svc.dueThoughts(context.Background())
	posted := h.discord.posted()
	if len(posted) != 2 || posted[1] != "what do the ugly neighbourhoods look like" {
		t.Fatalf("posted %q", posted)
	}
	day, _ := h.memory.Day(testGuild, clock)
	if last := day.Moments[len(day.Moments)-1].Text; !strings.Contains(last, "a moment later I added") {
		t.Errorf("the second thought was not remembered: %q", last)
	}
}

// If they answered in the meantime, the moment has passed.
func TestASecondThoughtIsDroppedOnceTheyAnswer(t *testing.T) {
	h := newHarness(t,
		appraisal(`"act":"reply","intent":"say it is clever","then":"ask about the name","then_after":40`),
		"that's clever, actually",
		"so what is it called",
	)
	h.say(t, "m1", "@Domme I made an app that shows code as a city", true)
	h.svc.conv.Record(testChannel, mind.Turn{UserID: bigM, Username: "Big M", Content: "thanks!", At: clock.Add(10 * time.Second)})

	h.svc.now = func() time.Time { return clock.Add(time.Minute) }
	h.svc.dueThoughts(context.Background())
	if got := h.discord.posted(); len(got) != 1 {
		t.Fatalf("a second thought landed after his answer: %q", got)
	}
}

func TestTwoParagraphsGoOutAsTwoMessages(t *testing.T) {
	h := newHarness(t, appraisal(`"act":"reply","intent":"hi"`), "huh. that's clever.\n\nwhat's it called")
	h.say(t, "m1", "@Domme look at this", true)
	posted := h.discord.posted()
	if len(posted) != 2 || posted[0] != "huh. that's clever" || posted[1] != "what's it called" {
		t.Fatalf("posted %q", posted)
	}
	if !h.svc.conv.Recent(testChannel)[len(h.svc.conv.Recent(testChannel))-1].FromBot {
		t.Error("the second part was not recorded as hers")
	}
}

func TestSecondThoughtsAreRationedPerHour(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < thoughtsPerHour; i++ {
		if !h.svc.mayThink(testGuild) {
			t.Fatalf("refused thought %d", i+1)
		}
		h.svc.thought(testGuild)
	}
	if h.svc.mayThink(testGuild) {
		t.Error("one more second thought than the hour allows")
	}
	h.svc.now = func() time.Time { return clock.Add(61 * time.Minute) }
	if !h.svc.mayThink(testGuild) {
		t.Error("the hour passed and she still may not")
	}
}

func TestTypingTakesAsLongAsTypingWould(t *testing.T) {
	if typingTime("ok") >= typingTime("a much longer message that takes a while to type out") {
		t.Error("a longer message is not slower to type")
	}
	if typingTime(strings.Repeat("x", 5000)) != typeMax {
		t.Error("a long message is held past the typing indicator")
	}
}

// Asked to, she looks back on today so far — and leaves today open, so the
// night still reflects on the whole of it rather than skipping a day that
// already has a summary.
func TestAskedSheReflectsOnTodayAndLeavesItOpen(t *testing.T) {
	h := newHarness(t,
		`{"summary":"a morning","lately":"quiet","people":[],"done":[],"threads":[]}`,
		`{"facts":[{"text":"hates mornings","line":1}]}`,
		`{"life":[{"text":"Big M keeps asking about the city thing","moments":[2]}],"wants":[]}`,
	)
	h.svc.mind.SelfFacts, h.svc.idleMind = true, true
	for _, mo := range []memory.Moment{
		{At: clock, Said: "m1", Text: `Big M: "morning"` + " → I said: " + `"i hate mornings"`},
		{At: clock, Text: `Big M: "how is the city thing"`},
	} {
		if err := h.memory.AddMoment(testGuild, mo); err != nil {
			t.Fatal(err)
		}
	}
	if !h.svc.ReflectNow(testGuild, true) {
		t.Fatal("the request was not taken")
	}
	if h.svc.ReflectNow(testGuild, true) {
		t.Error("a second request was taken while one was waiting")
	}
	h.svc.reflectAsked(context.Background(), <-h.svc.reflectAsk)

	if len(h.provider.sent) != 3 {
		t.Fatalf("%d calls, want the day, self-facts and life", len(h.provider.sent))
	}
	me, _ := h.memory.Me(testGuild)
	if len(me.Facts) != 1 {
		t.Errorf("self-facts %+v", me.Facts)
	}
	self, _ := h.memory.Self(testGuild)
	if len(self.Life) != 1 {
		t.Errorf("life %+v", self.Life)
	}
	day, _ := h.memory.Day(testGuild, clock)
	if day.Summary != "" {
		t.Errorf("today was closed with the summary %q", day.Summary)
	}
	if !h.svc.selfFactsDue(testGuild, clock) || !h.svc.lifeDue(testGuild, clock) {
		t.Error("today is marked as taken in; the night would skip the rest of it")
	}
}
