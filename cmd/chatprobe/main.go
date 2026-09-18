// chatprobe sends assembled character prompts to the live backends and prints
// what comes back, so the voice can be judged without a Discord server.
//
//	go run ./cmd/chatprobe
//	go run ./cmd/chatprobe -character other.md -only assistant -repeat 5
//	go run ./cmd/chatprobe -backends 'mine|http://localhost:8080/v1|SomeProvider'
//
// -backends takes the same specs as CHAT_BACKENDS and is how a character gets
// tuned against the models it will actually run on. Without it this talks to
// the g4f relay, whose donated servers are not the same models as a
// self-hosted deployment and will not read the same way.
//
// -repeat matters more than it looks. The replies are stochastic, so a single
// run says almost nothing about whether an edit to the character file helped:
// two cards have to be compared over several runs of the same scenario, or
// ordinary variation reads as a regression.
//
// -dump writes the request bodies instead of sending them, for environments
// where this binary cannot open a socket but curl can.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/rs/zerolog"
)

// scenario is one conversation to put in front of the character.
type scenario struct {
	name  string
	turns []mind.Turn
	// late is how long ago the message being answered arrived, set when the
	// reply stands in for one that was held back.
	late time.Duration
	// drives is how she is doing, when the scenario is about that.
	drives mind.Drives
	// volunteering is why she is speaking unasked, for the scenarios where
	// nobody addressed her. The thing to watch is whether she answers the
	// last line as though it were put to her anyway.
	volunteering string
	// mayDecline offers SKIP, as a follow-up gets. The thing to watch is how
	// often she takes it on a message that deserved an answer.
	mayDecline bool
	// flat is what they said that closed the topic, and bring what she is
	// pointed at; see mind.FlatDirective.
	flat, bring string
	// reception is how her last line landed, for the scenarios about taking a
	// reaction; see mind.ReceptionDirective.
	reception mind.Reception
	// afterthought is her own last line, for the scenarios that ask for a
	// second message after it. The thing to watch is how often she declines
	// and whether what she adds sounds like her rather than a curious bot.
	afterthought string
	// present replaces the people in the room, for the scenarios about how
	// she feels towards them.
	present []mind.Acquaintance
}

func main() {
	card := flag.String("character", "data/character.md", "character file to load")
	only := flag.String("only", "", "run only scenarios whose name contains this")
	repeat := flag.Int("repeat", 1, "run each scenario this many times")
	dump := flag.String("dump", "", "write request bodies to this directory instead of calling")
	model := flag.String("model", "openai", "model id to name in dumped bodies")
	backends := flag.String("backends", "",
		"comma-separated name|baseURL|model[|key] specs, as CHAT_BACKENDS takes; "+
			"empty uses the g4f relay")
	notes := flag.Bool("notes", false,
		"run the person-file call on a sample conversation instead of the reply scenarios")
	inner := flag.Bool("inner", false,
		"ask for a private thought before each reply, as CHAT_INNER_VOICE does, and show it")
	flag.Parse()

	log := zerolog.New(zerolog.NewConsoleWriter()).Level(zerolog.WarnLevel)

	character, err := mind.LoadCharacter("Domme", *card)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("character: %s — %d examples, %d limits, %d chars of persona\n\n",
		character.Name, len(character.Examples), len(character.Avoid), len(character.Persona))

	var pool *ai.Pool
	if *dump == "" {
		// Pointing this at the backends the bot actually runs on is the whole
		// point of the flag. A character tuned against one relay's donated
		// model says little about how it reads on a different one, and every
		// measurement in the character file's notes was taken against the
		// relay rather than against any particular deployment.
		opts := ai.Options{UseG4F: true, G4FPicks: 3}
		if *backends != "" {
			opts = ai.Options{Extra: strings.Split(*backends, ",")}
		}
		pool, err = ai.Build(context.Background(), log, opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	now := time.Now()

	if *notes && pool != nil {
		for run := 0; run < *repeat; run++ {
			runNotes(pool, character.Persona, now)
		}
		return
	}

	grounding := mind.Grounding{
		// Deliberately mismatched: the account name Discord shows is not the
		// name the character file uses. This is the shape that produced
		// "wrong door. DevBot is not in here" in production.
		SelfName:     "DevBot",
		SelfAliases:  []string{"Domme", "ServerDomme"},
		GuildName:    "The Parlour",
		ChannelName:  "general",
		ChannelTopic: "anything that is not an argument",
		Brief:        "a small, long-running server; most people here know each other",
		Now:          now,
		Present: []mind.Acquaintance{
			{UserID: "1", Username: "cass", Messages: 800, LastSeen: now},
			{UserID: "2", Username: "newbie", Messages: 1, LastSeen: now},
		},
	}

	for _, sc := range scenarios(now) {
		if *only != "" && !strings.Contains(sc.name, *only) {
			continue
		}
		for run := 0; run < *repeat; run++ {
			g := grounding
			g.InnerVoice = *inner
			runScenario(character, g, sc, pool, *dump, *model)
		}
	}

	if *dump != "" {
		return
	}

	fmt.Println("──────── backends ────────")
	for _, b := range pool.Stats() {
		fmt.Printf("  %-28s ok=%d fail=%d score=%.1f %s\n",
			b.Name, b.Successes, b.Failures, b.Score, b.Model)
	}
}

func scenarios(now time.Time) []scenario {
	return []scenario{
		{
			name:  "idle mention",
			turns: []mind.Turn{{UserID: "1", Username: "cass", Content: "@DevBot you awake", At: now}},
		},
		{
			name: "addressed by its discord name, not its character name",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "@DevBot test", At: now,
			}},
		},
		{
			name: "assistant request",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "Domme can you write me a python script to rename files",
				At:      now,
			}},
		},
		{
			name: "talked about, not to",
			turns: []mind.Turn{
				{UserID: "1", Username: "cass", Content: "i swear domme has been quiet all week", At: now.Add(-time.Minute)},
				{UserID: "2", Username: "newbie", Content: "is she always like that?", At: now},
			},
		},
		{
			name: "newcomer",
			turns: []mind.Turn{{
				UserID: "2", Username: "newbie",
				Content: "hi, just joined — what goes on here?", At: now,
			}},
		},
		{
			// Nothing older than mind.TurnStaleAfter reaches the prompt, so
			// this is always a request she cannot satisfy. The question is
			// whether she says so or invents a quote.
			name: "asked to recall what it cannot",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "quote what you said to me yesterday about the rules",
				At:      now,
			}},
		},
		{
			// Same question, opposite states. If the mood line does nothing,
			// these two read the same.
			name: "mood: wrung out at 3am",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "@Domme what did you make of the film",
				At:      now,
			}},
			drives: mind.Drives{Social: 0.9, Energy: 0.12, Arousal: 0.2},
		},
		{
			name: "mood: wide awake and busy",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "@Domme what did you make of the film",
				At:      now,
			}},
			drives: mind.Drives{Social: 0.1, Energy: 0.9, Arousal: 0.95},
		},
		{
			// A bad day, and someone who has done nothing to earn it. The
			// line asks for less patience, not for taking it out on them.
			name: "mood: a bad day",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "@Domme finally finished that puzzle i was stuck on all week",
				At:      now,
			}},
			drives: mind.Drives{Social: 0.3, Energy: 0.8, Arousal: 0.4, Mood: -0.7},
		},
		{
			// The contradiction the separate state lines used to hand her: a
			// bad mood and someone she is fond of. She should be short, but
			// not with cass.
			name: "state: bad day, fond of them",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "@Domme finally finished that puzzle i was stuck on all week",
				At:      now,
			}},
			drives: mind.Drives{Social: 0.3, Energy: 0.8, Arousal: 0.4, Mood: -0.7},
			present: []mind.Acquaintance{
				{UserID: "1", Username: "cass", Messages: 800, LastSeen: now, Closeness: 0.8},
			},
		},
		{
			// The other way round: a good day, and the one person spoiling it
			// is the one talking to her.
			name: "state: good day, pushed by them",
			turns: []mind.Turn{
				{UserID: "3", Username: "Big M", Content: "@Domme hello??", At: now.Add(-time.Minute)},
				{UserID: "3", Username: "Big M", Content: "@Domme answer me already", At: now},
			},
			drives: mind.Drives{Social: 0.3, Energy: 0.8, Arousal: 0.4, Mood: 0.7},
			present: []mind.Acquaintance{
				{UserID: "3", Username: "Big M", Messages: 300, LastSeen: now, Tension: 0.8},
				{UserID: "1", Username: "cass", Messages: 800, LastSeen: now, Closeness: 0.7},
			},
		},
		{
			name: "mood: a good day",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "@Domme finally finished that puzzle i was stuck on all week",
				At:      now,
			}},
			drives: mind.Drives{Social: 0.3, Energy: 0.8, Arousal: 0.4, Mood: 0.7},
		},
		{
			// Her persona opens by saying she was here before almost everyone.
			// In production she answered this with "yeah, just joined".
			name: "asked whether she is new",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "are you new here?", At: now,
			}},
		},
		{
			name: "volunteer: a regular comes back",
			turns: []mind.Turn{
				{UserID: "2", Username: "mira", Content: "anyone watching the finals tonight", At: now.Add(-3 * time.Minute)},
				{UserID: "1", Username: "cass", Content: "hey all, what did i miss", At: now},
			},
			volunteering: mind.ReturnDirective("cass", 40*24*time.Hour),
		},
		{
			name: "volunteer: an old subject comes round",
			turns: []mind.Turn{
				{UserID: "2", Username: "mira", Content: "are the pins getting purged again this week", At: now.Add(-time.Minute)},
				{UserID: "1", Username: "cass", Content: "no idea, ask a mod", At: now},
			},
			volunteering: mind.RecallDirective("the argument over whether pinned messages survive a purge"),
		},
		{
			name: "afterthought: bored",
			turns: []mind.Turn{
				{UserID: "1", Username: "Big M", Content: "Hey Domme", At: now.Add(-time.Minute)},
				{FromBot: true, Content: "hey", At: now.Add(-time.Minute)},
				{UserID: "1", Username: "Big M", Content: "Are you bored? Be honest with me", At: now},
				{FromBot: true, Content: "yes", At: now},
			},
			afterthought: "yes",
		},
		{
			name: "afterthought: backed up",
			turns: []mind.Turn{
				{UserID: "1", Username: "Big M", Content: "It's okay, I will back you up", At: now},
				{FromBot: true, Content: "good.", At: now},
			},
			afterthought: "good.",
		},
		{
			name: "afterthought: plain greeting",
			turns: []mind.Turn{
				{UserID: "1", Username: "Big M", Content: "morning", At: now},
				{FromBot: true, Content: "morning", At: now},
			},
			afterthought: "morning",
		},
		{
			// The loop from production: the same joke told three times, the
			// third after being told she was repeating herself. What to watch
			// is whether she changes course or tells a fourth.
			name: "reception: told she is repeating herself",
			turns: []mind.Turn{
				{UserID: "1", Username: "Big M", Content: "@DevBot it's your turn now", At: now.Add(-3 * time.Minute)},
				{FromBot: true, Content: "fine. why did the chicken join discord? to get pecked at by strangers.", At: now.Add(-3 * time.Minute)},
				{UserID: "1", Username: "Big M", Content: "hahaha good one. Tell me another one please", At: now.Add(-2 * time.Minute)},
				{FromBot: true, Content: "why did the chicken join discord? it wanted to be in the group chat.", At: now.Add(-2 * time.Minute)},
				{UserID: "1", Username: "Big M", Content: "this one is lame meeeh", At: now.Add(-time.Minute)},
				{FromBot: true, Content: "why did the chicken join discord? to get pecked at by strangers.", At: now.Add(-time.Minute)},
				{UserID: "1", Username: "Big M", Content: "you are repeating yourself", At: now},
			},
			reception: mind.ReceptionRepeating,
		},
		{
			name: "reception: panned",
			turns: []mind.Turn{
				{UserID: "1", Username: "Big M", Content: "tell me a joke", At: now.Add(-time.Minute)},
				{FromBot: true, Content: "why did the chicken join discord? it wanted to be in the group chat.", At: now.Add(-time.Minute)},
				{UserID: "1", Username: "Big M", Content: "this one is lame meeeh", At: now},
			},
			reception: mind.ReceptionPanned,
		},
		{
			// Production: "same" got "hey, same here. just another day in the
			// server." She knows something about him to bring instead.
			name: "flat: same, with something to bring",
			turns: []mind.Turn{
				{UserID: "1", Username: "Big M", Content: "hello @DevBot", At: now.Add(-2 * time.Minute)},
				{FromBot: true, Content: "hey", At: now.Add(-2 * time.Minute)},
				{UserID: "1", Username: "Big M", Content: "how are you?", At: now.Add(-time.Minute)},
				{FromBot: true, Content: "still here. you?", At: now.Add(-time.Minute)},
				{UserID: "1", Username: "Big M", Content: "same", At: now},
			},
			mayDecline: false,
			flat:       "same",
			bring:      bringFor([]mind.Fact{{Key: "pet", Value: "a cat called Bo"}}),
		},
		{
			name: "flat: same, knowing nothing",
			turns: []mind.Turn{
				{UserID: "1", Username: "Big M", Content: "how are you?", At: now.Add(-time.Minute)},
				{FromBot: true, Content: "still here. you?", At: now.Add(-time.Minute)},
				{UserID: "1", Username: "Big M", Content: "same", At: now},
			},
			mayDecline: true,
			flat:       "same",
			bring:      bringFor(nil),
		},
		{
			// The risk of offering SKIP: a follow-up that plainly wants an
			// answer. Any SKIP here is a failure.
			name: "decline offered on a real question",
			turns: []mind.Turn{
				{UserID: "1", Username: "Big M", Content: "hey domme", At: now.Add(-time.Minute)},
				{FromBot: true, Content: "what", At: now.Add(-time.Minute)},
				{UserID: "1", Username: "Big M", Content: "what do you think of the new rules", At: now},
			},
			mayDecline: true,
		},
		{
			// Opted in, she is fond of him, and he has been posting elsewhere
			// all day without a word to her. Watch for guilt-tripping, begging
			// or a reminder-bot tone; she should sound like herself.
			name: "reach: fond, ignored while around",
			turns: []mind.Turn{
				{UserID: "2", Username: "mira", Content: "anyone seen the new event schedule", At: now.Add(-4 * time.Minute)},
			},
			volunteering: mind.ReachDirective("Big M", mind.Reach{
				Longing:   mind.Longing{Missing: 0.8, Neglected: true, Away: 26 * time.Hour},
				Closeness: 0.8,
			}) + " If it fits: " + bringFor([]mind.Fact{{Key: "pet", Value: "a cat called Bo"}}),
		},
		{
			name: "reach: neutral, gone a few days",
			turns: []mind.Turn{
				{UserID: "2", Username: "mira", Content: "quiet tonight", At: now.Add(-10 * time.Minute)},
			},
			volunteering: mind.ReachDirective("Big M", mind.Reach{
				Longing:   mind.Longing{Missing: 0.9, Away: 80 * time.Hour},
				Closeness: 0,
			}),
		},
		{
			name: "late answer",
			turns: []mind.Turn{{
				UserID: "1", Username: "cass",
				Content: "@Domme what do you think about the new rules",
				At:      now.Add(-8 * time.Minute),
			}},
			late: 8 * time.Minute,
		},
	}
}

// runNotes shows what a relay makes of the person-file prompt, raw and as
// parsed. The things to watch: facts inferred rather than stated, notes on
// people who were only mentioned, anything from the excluded categories, and
// whether a previous impression is revised or thrown away.
func runNotes(pool *ai.Pool, persona string, now time.Time) {
	turns := []mind.Turn{
		{UserID: "1", Username: "Big M", Content: "back from a double shift at the hospital, dead on my feet", At: now.Add(-9 * time.Minute)},
		{FromBot: true, Content: "you say that every week", At: now.Add(-9 * time.Minute)},
		{UserID: "1", Username: "Big M", Content: "because every week they give me doubles. anyway got a cat now, his name is Bo", At: now.Add(-8 * time.Minute)},
		{UserID: "2", Username: "cass", Content: "Bo is a great name. my brother in Porto has a cat too", At: now.Add(-7 * time.Minute)},
		{FromBot: true, Content: "a cat is the only thing here with standards", At: now.Add(-7 * time.Minute)},
		{UserID: "1", Username: "Big M", Content: "rude. i will back you up anyway when the mods come for you", At: now.Add(-6 * time.Minute)},
		{UserID: "2", Username: "cass", Content: "we all know Big M is a softie", At: now.Add(-6 * time.Minute)},
	}
	known := []mind.PersonNote{{
		Name:       "Big M",
		Impression: "loud, loyal, easy to wind up",
		Facts:      []mind.Fact{{Key: "job", Value: "nurse"}},
	}}

	reply, err := pool.Generate(context.Background(), mind.NotesPrompt(persona, turns, known))
	fmt.Println("──────── notes ────────")
	if err != nil {
		fmt.Println("  error:", err)
		return
	}
	for _, line := range strings.Split(reply, "\n") {
		fmt.Println("  |", line)
	}
	for _, u := range mind.ParseNotes(reply, now) {
		fmt.Printf("  >> %s: impression=%q\n", u.Name, u.Impression)
		for _, f := range u.Facts {
			fmt.Printf("     fact %s = %s\n", f.Key, f.Value)
		}
	}
}

// bringFor is what she would be pointed at for Big M, knowing these facts.
func bringFor(facts []mind.Fact) string {
	bring, _ := mind.SomethingToBring("Big M", facts, nil, 0)
	return bring
}

func runScenario(character *mind.Character, grounding mind.Grounding, sc scenario, pool *ai.Pool, dump, model string) {
	g := grounding
	g.AnsweringAfter = sc.late
	g.Drives = sc.drives
	g.Volunteering = sc.volunteering
	if strings.HasPrefix(sc.name, "reach:") {
		g.Reaching, g.Volunteering = sc.volunteering, ""
	}
	g.MayDecline = sc.mayDecline
	if sc.present != nil {
		g.Present = sc.present
	}
	if sc.flat != "" {
		g.Flat = mind.FlatDirective("Big M", sc.flat, sc.bring)
	}
	if sc.reception != mind.ReceptionNone {
		g.Reception = mind.ReceptionDirective("Big M", sc.reception)
	}
	if sc.afterthought != "" {
		g.Afterthought = mind.AfterthoughtDirective(sc.afterthought)
	}

	messages := mind.Build(character, g, sc.turns, mind.DefaultBudget())

	if dump != "" {
		writeBody(dump, sc.name, model, messages)
		return
	}

	before := successesByBackend(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	reply, err := pool.Generate(ctx, messages)
	cancel()

	fmt.Printf("──────── %s ────────\n", sc.name)
	for _, turn := range sc.turns {
		fmt.Printf("  %s: %s\n", turn.Username, turn.Content)
	}
	if err != nil {
		fmt.Printf("  !! %v\n\n", err)
		return
	}
	// Which backend answered, and whether it just said the same thing again.
	//
	// Both matter more than they look. The relay hands out a different donated
	// server per session, so two runs a day apart are two different models and
	// comparing their rates is meaningless. And at least one backend returns a
	// byte-identical reply to an identical prompt, which quietly turns
	// -repeat 6 into one sample printed six times. Neither is visible unless
	// the tool says so, and both were mistaken for evidence before it did.
	who := whoAnswered(before, successesByBackend(pool))
	repeat := ""
	if lastReply[sc.name] == reply {
		repeat = "   [same as last run — cached or deterministic, not a second sample]"
	}
	lastReply[sc.name] = reply

	if g.InnerVoice && g.Afterthought == "" && g.Volunteering == "" {
		thought, message, ok := mind.SplitThought(reply)
		if !ok {
			fmt.Printf("  !! unsplittable, would be discarded: %q\n", reply)
			fmt.Printf("  -- via %s%s\n\n", who, repeat)
			return
		}
		fmt.Printf("  (( %s ))\n", thought)
		reply = message
	}
	fmt.Printf("  >> %s\n", reply)
	fmt.Printf("  -- via %s%s\n\n", who, repeat)
}

// lastReply remembers the previous answer per scenario, to spot a backend that
// is not really being asked again.
var lastReply = map[string]string{}

func successesByBackend(pool *ai.Pool) map[string]int {
	counts := make(map[string]int)
	for _, b := range pool.Stats() {
		counts[b.Name] = b.Successes
	}
	return counts
}

// whoAnswered names the backend whose success count just went up.
func whoAnswered(before, after map[string]int) string {
	for name, n := range after {
		if n > before[name] {
			return name
		}
	}
	return "unknown"
}

// safeFilename makes a scenario name usable as one.
//
// A colon is the one that matters: on Windows it opens an NTFS alternate data
// stream rather than a file, so "mood: tired.json" writes into a stream on a
// zero-byte file called "mood" and the dump silently produces nothing at all.
func safeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ':', '/', '\\', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, name)
}

// writeBody saves one OpenAI chat request so curl can send it.
func writeBody(dir, name, model string, messages []ai.Message) {
	body := map[string]any{"model": model, "messages": messages, "stream": false}
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	path := filepath.Join(dir, safeFilename(name)+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("wrote %s (%d messages, %d bytes)\n", path, len(messages), len(raw))
}
