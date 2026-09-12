package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

const (
	selfUserID  = "bot-1"
	testGuild   = "g1"
	testChannel = "c1"
)

func testStore(t *testing.T) *storage.Storage {
	t.Helper()
	s, err := storage.NewStorage(t.TempDir(), zerolog.Nop())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

// testSession is a session with only the state Observe reads. Nothing here
// touches the network.
func testSession() *discordgo.Session {
	state := discordgo.NewState()
	state.User = &discordgo.User{ID: selfUserID}
	return &discordgo.Session{State: state}
}

// newTestService wires a service whose speak-or-stay-quiet decision is fixed by
// roll, so the tests exercise routing rather than the random source.
func newTestService(t *testing.T, store *storage.Storage, roll float64) *Service {
	t.Helper()
	sess := testSession()
	return New(Deps{
		Character: &mind.Character{Name: "Domme", Persona: "someone"},
		Provider:  nil, // Observe never reaches a backend.
		Storage:   store,
		Session:   func() *discordgo.Session { return sess },
		Log:       zerolog.Nop(),
		Roll:      func() float64 { return roll },
	})
}

// message builds a MessageCreate as the gateway would deliver one.
func message(content string, mentionsBot bool) *discordgo.MessageCreate {
	m := &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "m1",
		GuildID:   testGuild,
		ChannelID: testChannel,
		Content:   content,
		Author:    &discordgo.User{ID: "u1", Username: "cass"},
	}}
	if mentionsBot {
		m.Mentions = []*discordgo.User{{ID: selfUserID}}
	}
	return m
}

// queued reports the approach the service enqueued, if any.
func queued(s *Service) (task, bool) {
	select {
	case t := <-s.work:
		return t, true
	default:
		return task{}, false
	}
}

func TestObserveIgnoresChannelsNobodyOptedIn(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)

	svc.Observe(testSession(), message("@Domme hello", true))

	if _, ok := queued(svc); ok {
		t.Error("replied in a channel that was never opted in")
	}
	if got := store.GetMindPerson(testGuild, "u1"); got != nil {
		t.Error("recorded a member from a channel that was never opted in")
	}
}

// Two personas in one channel answer each other forever, and every turn of it
// costs a backend request.
func TestObserveNeverAnswersAnotherBot(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	m := message("@Domme hello", true)
	m.Author.Bot = true
	svc.Observe(testSession(), m)

	if _, ok := queued(svc); ok {
		t.Error("answered another bot")
	}
}

func TestObserveIgnoresItsOwnMessages(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	m := message("something she said", false)
	m.Author.ID = selfUserID
	svc.Observe(testSession(), m)

	if _, ok := queued(svc); ok {
		t.Error("answered itself")
	}
}

func TestObserveRecordsEveryoneEvenWhenNotAddressed(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	svc.Observe(testSession(), message("nothing to do with her", false))

	if _, ok := queued(svc); ok {
		t.Error("answered a message that did not address her")
	}
	person := store.GetMindPerson(testGuild, "u1")
	if person == nil || person.Messages != 1 {
		t.Fatalf("member not recorded: %+v", person)
	}
	if got := svc.conv.Recent(testChannel); len(got) != 1 {
		t.Errorf("conversation holds %d turns, want the message recorded", len(got))
	}
}

func TestObserveClassifiesHowSheWasAddressed(t *testing.T) {
	cases := []struct {
		name   string
		build  func() *discordgo.MessageCreate
		want   mind.Trigger
		queued bool
	}{
		{
			name:   "direct mention",
			build:  func() *discordgo.MessageCreate { return message("@Domme hello", true) },
			want:   mind.TriggerMention,
			queued: true,
		},
		{
			name: "reply to something she said",
			build: func() *discordgo.MessageCreate {
				m := message("fair enough", false)
				m.ReferencedMessage = &discordgo.Message{Author: &discordgo.User{ID: selfUserID}}
				return m
			},
			want:   mind.TriggerReply,
			queued: true,
		},
		{
			name:   "talked about, not to",
			build:  func() *discordgo.MessageCreate { return message("domme would hate that", false) },
			want:   mind.TriggerAbout,
			queued: true,
		},
		{
			name: "reply to someone else",
			build: func() *discordgo.MessageCreate {
				m := message("fair enough", false)
				m.ReferencedMessage = &discordgo.Message{Author: &discordgo.User{ID: "someone-else"}}
				return m
			},
			queued: false,
		},
		{
			name:   "her name inside a longer word",
			build:  func() *discordgo.MessageCreate { return message("the dommeish vibe", false) },
			queued: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := testStore(t)
			if err := store.AddChatChannel(testGuild, testChannel); err != nil {
				t.Fatalf("AddChatChannel: %v", err)
			}
			// Roll 0 always speaks, so anything queued reflects classification.
			svc := newTestService(t, store, 0)

			svc.Observe(testSession(), tc.build())

			got, ok := queued(svc)
			if ok != tc.queued {
				t.Fatalf("queued = %v, want %v", ok, tc.queued)
			}
			if ok && got.item.Trigger != tc.want {
				t.Errorf("trigger = %q, want %q", got.item.Trigger, tc.want)
			}
		})
	}
}

// Ignoring someone's first approach is a bot that appears not to work, so the
// rail has to hold through the service and not only in mind.Decide.
func TestObserveAlwaysAnswersAFirstApproachEvenOnTheWorstRoll(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0.9999)

	svc.Observe(testSession(), message("domme?", false))

	if _, ok := queued(svc); !ok {
		t.Error("ignored a first approach")
	}
}

func TestObserveNeverIgnoresTheSamePersonTwiceRunning(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0.9999)

	// First approach is always answered.
	svc.Observe(testSession(), message("domme?", false))
	if _, ok := queued(svc); !ok {
		t.Fatal("first approach was not answered")
	}

	// Second is ignored on this roll.
	svc.Observe(testSession(), message("domme, again", false))
	if _, ok := queued(svc); ok {
		t.Fatal("second approach was answered; the roll should have ignored it")
	}

	// Third must be answered regardless of the roll.
	svc.Observe(testSession(), message("domme, still", false))
	if _, ok := queued(svc); !ok {
		t.Error("ignored the same person twice running")
	}
}

// A busy moment has to produce a late answer rather than no answer, which is
// the same path a failed backend takes.
func TestObserveDefersWhenThereIsNoRoomToQueue(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)
	svc.work = make(chan task) // nothing can be enqueued

	svc.Observe(testSession(), message("@Domme hello", true))

	if svc.deferrals.Len() != 1 {
		t.Errorf("deferrals hold %d approaches, want the dropped one held", svc.deferrals.Len())
	}
}

func TestObserveCarriesWhatAReplyNeeds(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	svc.Observe(testSession(), message("@Domme hello", true))

	got, ok := queued(svc)
	if !ok {
		t.Fatal("nothing was queued")
	}
	if got.item.MessageID != "m1" {
		t.Errorf("MessageID = %q, want the message being answered", got.item.MessageID)
	}
	if got.item.GuildID != testGuild || got.item.ChannelID != testChannel {
		t.Errorf("approach lost its location: %+v", got.item)
	}
	if got.late {
		t.Error("a fresh approach is marked late")
	}
	if got.item.FormedAt.IsZero() {
		t.Error("approach has no time, so it can never expire")
	}
}

// She should call people what everyone else in the channel sees.
func TestDisplayNamePrefersTheGuildNickname(t *testing.T) {
	m := message("hi", false)
	m.Author.GlobalName = "Cassandra"
	if got := displayName(m); got != "Cassandra" {
		t.Errorf("displayName = %q, want the global name", got)
	}

	m.Member = &discordgo.Member{Nick: "Cass"}
	if got := displayName(m); got != "Cass" {
		t.Errorf("displayName = %q, want the guild nickname", got)
	}
}

func TestLastSpokeAtFindsHerOwnLastTurn(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)

	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{Username: "cass", Content: "a", At: now.Add(-2 * time.Minute)})
	svc.conv.Record(testChannel, mind.Turn{Content: "b", At: now.Add(-time.Minute), FromBot: true})
	svc.conv.Record(testChannel, mind.Turn{Username: "cass", Content: "c", At: now})

	if got := svc.lastSpokeAt(testChannel); !got.Equal(now.Add(-time.Minute)) {
		t.Errorf("lastSpokeAt = %v, want her own turn at %v", got, now.Add(-time.Minute))
	}

	if got := svc.lastSpokeAt("silent-channel"); !got.IsZero() {
		t.Errorf("lastSpokeAt = %v for a channel she never spoke in, want zero", got)
	}
}

// discordNamedSession is a session whose account name differs from whatever
// CHAT_NAME says, which is the situation that produced "wrong door".
func discordNamedSession(t *testing.T, username, nick string) *discordgo.Session {
	t.Helper()

	state := discordgo.NewState()
	state.User = &discordgo.User{ID: selfUserID, Username: username}
	if err := state.GuildAdd(&discordgo.Guild{ID: testGuild}); err != nil {
		t.Fatalf("GuildAdd: %v", err)
	}
	if nick != "" {
		if err := state.MemberAdd(&discordgo.Member{
			GuildID: testGuild,
			Nick:    nick,
			User:    &discordgo.User{ID: selfUserID, Username: username},
		}); err != nil {
			t.Fatalf("MemberAdd: %v", err)
		}
	}
	return &discordgo.Session{State: state}
}

func serviceNamed(t *testing.T, store *storage.Storage, configured ...string) *Service {
	t.Helper()
	return New(Deps{
		Character: &mind.Character{Name: configured[0], Persona: "someone"},
		Storage:   store,
		Session:   func() *discordgo.Session { return nil },
		Log:       zerolog.Nop(),
		Names:     configured,
		Roll:      func() float64 { return 0 },
	})
}

// Discord expands a mention to the account username, so a bot configured as
// "Dev" but named DevBot saw "@DevBot test", believed DevBot was someone else,
// and replied "wrong door. DevBot is not in here". It has to answer to the
// name Discord gives it, whatever the configuration says.
func TestObserveAnswersToItsDiscordNameNotOnlyItsConfiguredOne(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := serviceNamed(t, store, "Dev")
	sess := discordNamedSession(t, "DevBot", "")

	// No @mention: this is the name-drop path, which is the one that reads the
	// text rather than the mention list. "DevBot is being quiet today" is a
	// remark to the room, so it should route to TriggerAbout — what is being
	// tested here is that the name was recognised at all.
	svc.Observe(sess, message("DevBot is being quiet today", false))

	got, ok := queued(svc)
	if !ok {
		t.Fatal("did not recognise its own Discord name")
	}
	if got.item.Trigger != mind.TriggerAbout {
		t.Errorf("trigger = %q, want %q", got.item.Trigger, mind.TriggerAbout)
	}
}

func TestObserveAnswersToItsPerGuildNickname(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := serviceNamed(t, store, "Dev")
	sess := discordNamedSession(t, "DevBot", "Vera")

	svc.Observe(sess, message("ask Vera about it", false))

	if _, ok := queued(svc); !ok {
		t.Error("did not recognise the nickname members actually see")
	}
}

// A comma-separated CHAT_NAME is what an operator reaches for, and every
// spelling in it has to work.
func TestObserveAnswersToEverySpellingInTheConfiguredName(t *testing.T) {
	for _, spelling := range []string{"ServerDomme", "Server-Domme", "Server Domme"} {
		t.Run(spelling, func(t *testing.T) {
			store := testStore(t)
			if err := store.AddChatChannel(testGuild, testChannel); err != nil {
				t.Fatalf("AddChatChannel: %v", err)
			}
			svc := serviceNamed(t, store, "ServerDomme", "Server-Domme", " Server Domme ")
			sess := discordNamedSession(t, "ServerDomme", "")

			svc.Observe(sess, message("is "+spelling+" around", false))

			if _, ok := queued(svc); !ok {
				t.Errorf("did not answer to %q", spelling)
			}
		})
	}
}

func TestDisplayNamePrefersNicknameThenAccountName(t *testing.T) {
	svc := serviceNamed(t, testStore(t), "Dev")

	withNick := discordNamedSession(t, "DevBot", "Vera")
	if got := svc.DisplayName(withNick, testGuild); got != "Vera" {
		t.Errorf("DisplayName = %q, want the guild nickname", got)
	}

	withoutNick := discordNamedSession(t, "DevBot", "")
	if got := svc.DisplayName(withoutNick, testGuild); got != "DevBot" {
		t.Errorf("DisplayName = %q, want the account name", got)
	}

	// Falls back to configuration only when Discord tells us nothing.
	bare := &discordgo.Session{State: discordgo.NewState()}
	if got := svc.DisplayName(bare, testGuild); got != "Dev" {
		t.Errorf("DisplayName = %q, want the configured name as a fallback", got)
	}
}

// botTurn is one of her own messages, as speak records it after sending.
func botTurn(id, content string, at time.Time) mind.Turn {
	return mind.Turn{Content: content, At: at, FromBot: true, MessageID: id}
}

// replyTo builds a message that is a Discord reply to messageID. It carries no
// ReferencedMessage on purpose: discordgo documents that field as best-effort
// — "the backend did not attempt to fetch the message that was being replied
// to" — and a reply arriving without it was being silently ignored.
func replyTo(messageID, content string) *discordgo.MessageCreate {
	m := message(content, false)
	m.ID = "m-reply"
	m.MessageReference = &discordgo.MessageReference{
		MessageID: messageID,
		ChannelID: testChannel,
		GuildID:   testGuild,
	}
	return m
}

func TestObserveAnswersAReplyEvenWithoutTheReferencedMessage(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "you here?", At: now.Add(-time.Minute)})
	svc.conv.Record(testChannel, botTurn("m-hers", "here. what do you need", now.Add(-30*time.Second)))

	svc.Observe(testSession(), replyTo("m-hers", "wanted to know how are you"))

	got, ok := queued(svc)
	if !ok {
		t.Fatal("ignored a Discord reply to her own message")
	}
	if got.item.Trigger != mind.TriggerReply {
		t.Errorf("trigger = %q, want %q", got.item.Trigger, mind.TriggerReply)
	}
}

func TestObserveIgnoresAReplyToSomeoneElse(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u9", Username: "someone", Content: "hi", At: now, MessageID: "m-theirs"})

	svc.Observe(testSession(), replyTo("m-theirs", "agreed"))

	if _, ok := queued(svc); ok {
		t.Error("answered a reply aimed at another member")
	}
}

// People stop tagging you once a conversation is running. Answering the first
// message and then going deaf is how a bot gives itself away.
func TestObserveAnswersAnUntaggedFollowUp(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "you here?", At: now.Add(-time.Minute)})
	svc.conv.Record(testChannel, botTurn("m-hers", "here. what do you need", now.Add(-20*time.Second)))

	svc.Observe(testSession(), message("what are you thinking about", false))

	got, ok := queued(svc)
	if !ok {
		t.Fatal("ignored the next message from the person she was talking to")
	}
	if got.item.Trigger != mind.TriggerFollowUp {
		t.Errorf("trigger = %q, want %q", got.item.Trigger, mind.TriggerFollowUp)
	}
}

// Once someone else has spoken the thread is no longer hers to assume.
func TestObserveDoesNotFollowUpAfterSomeoneElseSpoke(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "you here?", At: now.Add(-time.Minute)})
	svc.conv.Record(testChannel, botTurn("m-hers", "here", now.Add(-40*time.Second)))
	svc.conv.Record(testChannel, mind.Turn{UserID: "u9", Username: "bystander", Content: "unrelated", At: now.Add(-20 * time.Second)})

	svc.Observe(testSession(), message("anyway as i was saying", false))

	if _, ok := queued(svc); ok {
		t.Error("joined a conversation she was no longer the last voice in")
	}
}

// A bystander's remark is not addressed to her just because she spoke last.
func TestObserveDoesNotFollowUpForADifferentPerson(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u9", Username: "someone", Content: "hey", At: now.Add(-time.Minute)})
	svc.conv.Record(testChannel, botTurn("m-hers", "what", now.Add(-20*time.Second)))

	// message() is from u1, who is not the person she was answering.
	svc.Observe(testSession(), message("totally different thing", false))

	if _, ok := queued(svc); ok {
		t.Error("treated a bystander's message as a follow-up")
	}
}

// A reply arriving against a conversation everyone has left is not a
// follow-up, it is a new approach.
func TestObserveDoesNotFollowUpAfterTheExchangeWentCold(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	now := time.Now()
	cold := now.Add(-2 * mind.DefaultAttention().EngagedWindow)
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "you here?", At: cold.Add(-time.Minute)})
	svc.conv.Record(testChannel, botTurn("m-hers", "here", cold))

	svc.Observe(testSession(), message("so about that", false))

	if _, ok := queued(svc); ok {
		t.Error("followed up on an exchange that had gone cold")
	}
}

// Once an approach is abandoned it must leave nothing behind, or the retry
// loop keeps finding it and the channel keeps seeing her start to type.
func TestHoldAbandonsAnApproachThatHasHadEnoughAttempts(t *testing.T) {
	svc := newTestService(t, testStore(t), 0)

	spent := mind.Deferred{
		GuildID:   testGuild,
		ChannelID: testChannel,
		MessageID: "m1",
		FormedAt:  time.Now(),
		Attempts:  mind.MaxDeferralAttempts,
	}
	svc.hold(task{item: spent}, "generate failed")

	if svc.deferrals.Len() != 0 {
		t.Errorf("a spent approach was put back: %d held", svc.deferrals.Len())
	}
}

func TestHoldKeepsAnApproachWithAttemptsLeft(t *testing.T) {
	svc := newTestService(t, testStore(t), 0)

	fresh := mind.Deferred{
		GuildID:   testGuild,
		ChannelID: testChannel,
		MessageID: "m1",
		FormedAt:  time.Now(),
	}
	svc.hold(task{item: fresh}, "generate failed")

	if svc.deferrals.Len() != 1 {
		t.Errorf("a fresh approach was dropped: %d held", svc.deferrals.Len())
	}
}

// A first backend that hangs until its own deadline must still leave the
// second one room to answer, rather than being cancelled on the way in.
func TestGenerateTimeoutLeavesRoomForFailover(t *testing.T) {
	slow := New(Deps{
		Character:      &mind.Character{Name: "X", Persona: "someone"},
		Storage:        testStore(t),
		Session:        func() *discordgo.Session { return nil },
		Log:            zerolog.Nop(),
		RequestTimeout: 120 * time.Second,
	})
	if slow.generateTimeout < 2*120*time.Second {
		t.Errorf("generateTimeout = %v, want at least two backend deadlines", slow.generateTimeout)
	}

	// A short per-request timeout must not shrink the overall budget below
	// the default.
	quick := New(Deps{
		Character:      &mind.Character{Name: "X", Persona: "someone"},
		Storage:        testStore(t),
		Session:        func() *discordgo.Session { return nil },
		Log:            zerolog.Nop(),
		RequestTimeout: 5 * time.Second,
	})
	if quick.generateTimeout != defaultGenerateTimeout {
		t.Errorf("generateTimeout = %v, want the default %v", quick.generateTimeout, defaultGenerateTimeout)
	}
}

func historyMessage(id, userID, name, content string, at time.Time, bot bool) *discordgo.Message {
	return &discordgo.Message{
		ID:        id,
		ChannelID: testChannel,
		GuildID:   testGuild,
		Content:   content,
		Timestamp: at,
		Author:    &discordgo.User{ID: userID, Username: name, Bot: bot},
	}
}

// Discord returns newest first; the prompt reads oldest first.
func TestHistoryToTurnsReversesIntoChronologicalOrder(t *testing.T) {
	svc := newTestService(t, testStore(t), 0)
	now := time.Now()

	turns := svc.historyToTurns(testSession(), []*discordgo.Message{
		historyMessage("m3", "u1", "cass", "third", now, false),
		historyMessage("m2", "u1", "cass", "second", now.Add(-time.Minute), false),
		historyMessage("m1", "u1", "cass", "first", now.Add(-2*time.Minute), false),
	})

	want := []string{"first", "second", "third"}
	if len(turns) != len(want) {
		t.Fatalf("got %d turns, want %d", len(turns), len(want))
	}
	for i := range want {
		if turns[i].Content != want[i] {
			t.Errorf("turn %d = %q, want %q", i, turns[i].Content, want[i])
		}
	}
}

// Her own past messages are context; another bot's output is noise, and
// answering it is how two bots talk to each other forever.
func TestHistoryToTurnsKeepsItsOwnMessagesAndDropsOtherBots(t *testing.T) {
	svc := newTestService(t, testStore(t), 0)
	now := time.Now()

	turns := svc.historyToTurns(testSession(), []*discordgo.Message{
		historyMessage("m3", "other-bot", "MEE6", "level up!", now, true),
		historyMessage("m2", selfUserID, "Domme", "unfortunately", now.Add(-time.Minute), true),
		historyMessage("m1", "u1", "cass", "you up", now.Add(-2*time.Minute), false),
	})

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2 (the other bot dropped):\n%+v", len(turns), turns)
	}
	if turns[0].Content != "you up" || turns[0].FromBot {
		t.Errorf("first turn should be the human's: %+v", turns[0])
	}
	if turns[1].Content != "unfortunately" || !turns[1].FromBot {
		t.Errorf("second turn should be her own, marked FromBot: %+v", turns[1])
	}
}

// An attachment or sticker with no text has nothing in it for a language
// model to read.
func TestHistoryToTurnsDropsEmptyMessages(t *testing.T) {
	svc := newTestService(t, testStore(t), 0)

	turns := svc.historyToTurns(testSession(), []*discordgo.Message{
		historyMessage("m1", "u1", "cass", "", time.Now(), false),
	})

	if len(turns) != 0 {
		t.Errorf("kept a message with no text: %+v", turns)
	}
}

func TestBackfillDoesNothingOnceAttempted(t *testing.T) {
	svc := newTestService(t, testStore(t), 0)

	// A nil session is the cheapest way to prove no fetch happens: a real
	// attempt would dereference it.
	svc.conv.Seed(testChannel, nil)
	svc.backfill(nil, testChannel)

	if svc.conv.NeedsSeed(testChannel) {
		t.Error("channel still wants seeding")
	}
}

// The whole point of the split: the same name, two intentions, two sets of
// odds.
func TestObserveSeparatesBeingAddressedFromBeingDiscussed(t *testing.T) {
	cases := map[string]struct {
		text string
		want mind.Trigger
	}{
		"spoken to by name":   {"Domme, what do you make of this", mind.TriggerNamed},
		"question naming her": {"is Domme around?", mind.TriggerNamed},
		"remarked upon":       {"Domme would hate this", mind.TriggerAbout},
		"discussed":           {"i swear Domme has been quiet all week", mind.TriggerAbout},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := testStore(t)
			if err := store.AddChatChannel(testGuild, testChannel); err != nil {
				t.Fatalf("AddChatChannel: %v", err)
			}
			svc := newTestService(t, store, 0)

			svc.Observe(testSession(), message(tc.text, false))

			got, ok := queued(svc)
			if !ok {
				t.Fatalf("%q produced no approach at all", tc.text)
			}
			if got.item.Trigger != tc.want {
				t.Errorf("%q = %q, want %q", tc.text, got.item.Trigger, tc.want)
			}
		})
	}
}

// rememberingService wires a service to a provider that answers summaries with
// a fixed reply, so the writer can be driven without a backend.
func rememberingService(t *testing.T, store *storage.Storage, reply string, err error) *Service {
	t.Helper()
	return New(Deps{
		Character: &mind.Character{Name: "Domme", Persona: "someone"},
		Provider:  stubProvider{reply: reply, err: err},
		Storage:   store,
		Session:   func() *discordgo.Session { return nil },
		Log:       zerolog.Nop(),
		Roll:      func() float64 { return 0 },
	})
}

type stubProvider struct {
	reply string
	err   error
}

func (p stubProvider) Generate(context.Context, []ai.Message) (string, error) {
	return p.reply, p.err
}

func conversation(n int, at time.Time) []mind.Turn {
	turns := make([]mind.Turn, 0, n)
	for i := 0; i < n; i++ {
		turns = append(turns, mind.Turn{
			UserID: "u1", Username: "cass", Content: "something said",
			At: at.Add(time.Duration(i) * time.Second),
		})
	}
	return turns
}

// optIn lets the channel into the persona. Every memory-writing test needs it
// explicitly: without it the opt-in guard skips the channel, and a test that
// expects nothing to be written would pass for the wrong reason.
func optIn(t *testing.T, store *storage.Storage) {
	t.Helper()
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
}

func TestRememberSettledWritesAMemory(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := rememberingService(t, store,
		"GIST: an argument about pins\nDETAIL: it ran long and nobody conceded.", nil)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(worthRemembering, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}

	svc.rememberSettled(context.Background())

	got := store.MindMemories(testGuild, testChannel)
	if len(got) != 1 {
		t.Fatalf("stored %d memories, want 1", len(got))
	}
	if got[0].Gist != "an argument about pins" {
		t.Errorf("gist = %q", got[0].Gist)
	}
	if len(got[0].People) == 0 {
		t.Error("nobody was recorded as having been there")
	}
}

// Summarising mid-conversation produces a memory of half an argument.
func TestRememberSkipsAConversationStillInProgress(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := rememberingService(t, store, "GIST: too early\n", nil)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(worthRemembering, time.Now()) {
		svc.conv.Record(testChannel, turn)
	}

	svc.rememberSettled(context.Background())

	if got := store.MindMemories(testGuild, testChannel); len(got) != 0 {
		t.Errorf("remembered a live conversation: %+v", got)
	}
}

func TestRememberSkipsAPassingExchange(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := rememberingService(t, store, "GIST: not worth it\n", nil)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(2, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}

	svc.rememberSettled(context.Background())

	if got := store.MindMemories(testGuild, testChannel); len(got) != 0 {
		t.Errorf("remembered two lines of small talk: %+v", got)
	}
}

// Deriving this from what is stored rather than from a marker in memory is
// what stops a restart paying for the same memory twice.
func TestRememberDoesNotWriteTheSameConversationTwice(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := rememberingService(t, store, "GIST: the same thing\n", nil)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(worthRemembering, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}

	svc.rememberSettled(context.Background())
	svc.rememberSettled(context.Background())

	if got := store.MindMemories(testGuild, testChannel); len(got) != 1 {
		t.Errorf("stored %d memories for one conversation", len(got))
	}
}

// A memory nobody can read is not an error. Nobody is waiting on it.
func TestRememberSurvivesAnUnreadableReply(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := rememberingService(t, store, "I'm sorry, I can't help with that.", nil)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(worthRemembering, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}

	svc.rememberSettled(context.Background())

	if got := store.MindMemories(testGuild, testChannel); len(got) != 0 {
		t.Errorf("stored something from an unparseable reply: %+v", got)
	}
}

func TestRememberSurvivesABackendFailure(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := rememberingService(t, store, "", ai.ErrNoBackend)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(worthRemembering, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}

	svc.rememberSettled(context.Background())

	if got := store.MindMemories(testGuild, testChannel); len(got) != 0 {
		t.Errorf("stored a memory despite the backend failing: %+v", got)
	}
}

// A channel the bot has not seen a message in this run has no guild to file
// the memory under.
func TestRememberSkipsChannelsWithNoKnownGuild(t *testing.T) {
	store := testStore(t)
	svc := rememberingService(t, store, "GIST: orphaned\n", nil)

	for _, turn := range conversation(worthRemembering, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}

	svc.rememberSettled(context.Background())

	if got := store.MindMemories(testGuild, testChannel); len(got) != 0 {
		t.Errorf("filed a memory under a guild it could not know: %+v", got)
	}
}

// Summarising sends a channel's contents to a third-party relay, which is the
// exact thing the opt-in governs — and the conversation outlives the opt-in,
// because silencing a channel leaves its turns in the buffer.
func TestRememberIgnoresAChannelThatIsNoLongerOptedIn(t *testing.T) {
	store := testStore(t)
	svc := rememberingService(t, store, "GIST: should never be written\n", nil)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(worthRemembering, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}

	// Never opted in at all: the store has no record of this channel.
	svc.rememberSettled(context.Background())

	if got := store.MindMemories(testGuild, testChannel); len(got) != 0 {
		t.Errorf("summarised a channel it was not let into: %+v", got)
	}
}

func TestRememberWritesOnceTheChannelIsOptedIn(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := rememberingService(t, store, "GIST: a real conversation\n", nil)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(worthRemembering, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}

	svc.rememberSettled(context.Background())

	if got := store.MindMemories(testGuild, testChannel); len(got) != 1 {
		t.Errorf("stored %d memories, want 1", len(got))
	}
}

// Being told to stop reading a channel has to take the conversation with it.
func TestForgetDropsTheConversationAndTheGuildMapping(t *testing.T) {
	svc := newTestService(t, testStore(t), 0)

	svc.noteGuild(testGuild, testChannel)
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "hi", At: time.Now()})

	svc.Forget(testChannel)

	if got := svc.conv.Recent(testChannel); len(got) != 0 {
		t.Errorf("still holding %d turns after being silenced", len(got))
	}
	if got := svc.guildOf(testChannel); got != "" {
		t.Errorf("still maps the channel to guild %q", got)
	}
}

// Pushing again straight after being passed over is what raises irritation —
// countable behaviour, not a judgement about tone.
func TestIrritationRisesWhenSomeonePressesAfterBeingIgnored(t *testing.T) {
	store := testStore(t)
	optIn(t, store)

	// Roll of 1 means every chance fails, so the first approach is ignored by
	// the odds and not by a rail.
	svc := newTestService(t, store, 1)

	// First approach is always answered by the rail, so it takes two to get
	// into the ignored state the pester check needs.
	svc.Observe(testSession(), message("@Domme hello", true))
	queued(svc) // clear the slot so the next approach is not queue-blocked
	svc.Observe(testSession(), message("@Domme still there", true))
	queued(svc) // clear the slot so the next approach is not queue-blocked
	svc.Observe(testSession(), message("@Domme answer me", true))
	queued(svc) // clear the slot so the next approach is not queue-blocked

	person := store.GetMindPerson(testGuild, "u1")
	if person == nil {
		t.Fatal("nobody was recorded at all")
	}
	if person.Irritation <= 0 {
		t.Errorf("pushing after being ignored left irritation at %.2f", person.Irritation)
	}
}

// Someone else speaking to her while she is short with one member should find
// her ordinary.
func TestIrritationIsHeldAgainstThePersonNotTheRoom(t *testing.T) {
	store := testStore(t)
	if err := store.IrritateMindPerson(testGuild, "u1", 0.9, time.Now()); err != nil {
		t.Fatalf("IrritateMindPerson: %v", err)
	}
	svc := newTestService(t, store, 0)

	if got := svc.irritationWith(testGuild, "u1", time.Now()); got < 0.5 {
		t.Errorf("irritation with the person who caused it = %.2f", got)
	}
	if got := svc.irritationWith(testGuild, "u2", time.Now()); got != 0 {
		t.Errorf("a bystander inherited %.2f of it", got)
	}
}

func TestIrritationFadesWithoutBeingTouched(t *testing.T) {
	store := testStore(t)
	long := time.Now().Add(-24 * time.Hour)
	if err := store.IrritateMindPerson(testGuild, "u1", 0.9, long); err != nil {
		t.Fatalf("IrritateMindPerson: %v", err)
	}
	svc := newTestService(t, store, 0)

	if got := svc.irritationWith(testGuild, "u1", time.Now()); got != 0 {
		t.Errorf("still annoyed a day later at %.2f", got)
	}
}

// Irritation on its own is a number. Asked what is wrong, she needs something
// to point at.
func TestIrritationLeavesARememberedCause(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := newTestService(t, store, 1)

	svc.Observe(testSession(), message("@Domme hello", true))
	queued(svc)
	svc.Observe(testSession(), message("@Domme still there", true))
	queued(svc)
	svc.Observe(testSession(), message("@Domme answer me", true))
	queued(svc)

	memories := store.MindMemories(testGuild, testChannel)
	if len(memories) == 0 {
		t.Fatal("she is annoyed and remembers nothing about why")
	}
	if !strings.Contains(memories[0].Gist, "pushing") {
		t.Errorf("the memory does not describe what happened: %q", memories[0].Gist)
	}
	if len(memories[0].People) == 0 || memories[0].People[0] != "u1" {
		t.Errorf("the memory is not attributed to anyone: %+v", memories[0].People)
	}
}

// One episode, one memory — not one per push, which would fill the store with
// the same sentence.
func TestIrritationRemembersTheEpisodeOnce(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := newTestService(t, store, 1)

	for i := 0; i < 6; i++ {
		svc.Observe(testSession(), message("@Domme answer me", true))
		queued(svc)
	}

	var pushes int
	for _, m := range store.MindMemories(testGuild, testChannel) {
		if strings.Contains(m.Gist, "pushing") {
			pushes++
		}
	}
	if pushes != 1 {
		t.Errorf("recorded %d memories for one episode", pushes)
	}
}

// A four-way row that went badly does not say who made it go badly, and a
// model asked "who was unpleasant" answers that worse than not asking.
func TestToneOnlyBecomesPersonalWithOnePersonInTheRoom(t *testing.T) {
	now := time.Now()

	t.Run("one person", func(t *testing.T) {
		store := testStore(t)
		svc := newTestService(t, store, 0)

		svc.takeItPersonally(testGuild, []string{"u1"}, mind.ToneHostile, now)

		if got := svc.irritationWith(testGuild, "u1", now); got <= 0 {
			t.Errorf("a hostile one-to-one left irritation at %.2f", got)
		}
	})

	t.Run("a crowd", func(t *testing.T) {
		store := testStore(t)
		svc := newTestService(t, store, 0)

		svc.takeItPersonally(testGuild, []string{"u1", "u2", "u3"}, mind.ToneHostile, now)

		for _, id := range []string{"u1", "u2", "u3"} {
			if got := svc.irritationWith(testGuild, id, now); got != 0 {
				t.Errorf("%s was blamed for a group row: %.2f", id, got)
			}
		}
	})

	t.Run("a pleasant conversation", func(t *testing.T) {
		store := testStore(t)
		svc := newTestService(t, store, 0)

		svc.takeItPersonally(testGuild, []string{"u1"}, mind.ToneWarm, now)

		if got := svc.irritationWith(testGuild, "u1", now); got != 0 {
			t.Errorf("a warm conversation made her cross: %.2f", got)
		}
	})
}

func TestRememberedToneLengthensTheMemory(t *testing.T) {
	store := testStore(t)
	optIn(t, store)
	svc := rememberingService(t, store,
		"GIST: the row\nDETAIL: it went badly.\nTONE: hostile", nil)

	svc.noteGuild(testGuild, testChannel)
	for _, turn := range conversation(worthRemembering, time.Now().Add(-settleFor-time.Minute)) {
		svc.conv.Record(testChannel, turn)
	}
	svc.rememberSettled(context.Background())

	got := store.MindMemories(testGuild, testChannel)
	if len(got) != 1 {
		t.Fatalf("stored %d memories", len(got))
	}

	plain := mind.WeighMoment(conversation(worthRemembering, time.Now()))
	if got[0].Weight <= plain {
		t.Errorf("a hostile conversation weighed %.2f, no more than an ordinary %.2f",
			got[0].Weight, plain)
	}
}
