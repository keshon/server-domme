// chatprobe sends assembled character prompts to the live backends and prints
// what comes back, so the voice can be judged without a Discord server.
//
//	go run ./cmd/chatprobe
//	go run ./cmd/chatprobe -character other.md -only assistant -repeat 5
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
}

func main() {
	card := flag.String("character", "data/character.md", "character file to load")
	only := flag.String("only", "", "run only scenarios whose name contains this")
	repeat := flag.Int("repeat", 1, "run each scenario this many times")
	dump := flag.String("dump", "", "write request bodies to this directory instead of calling")
	model := flag.String("model", "openai", "model id to name in dumped bodies")
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
		pool, err = ai.Build(context.Background(), log, ai.Options{UseG4F: true, G4FPicks: 3})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	now := time.Now()
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
			runScenario(character, grounding, sc, pool, *dump, *model)
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

func runScenario(character *mind.Character, grounding mind.Grounding, sc scenario, pool *ai.Pool, dump, model string) {
	g := grounding
	g.AnsweringAfter = sc.late

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
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("wrote %s (%d messages, %d bytes)\n", path, len(messages), len(raw))
}
