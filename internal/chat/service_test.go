package chat

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
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
