// chatprobe replays a conversation copied out of Discord through the persona,
// so a change to her can be judged against a real exchange rather than a
// made-up one.
//
//	go run ./cmd/chatprobe -log temp/log.txt
//	go run ./cmd/chatprobe -log temp/log.txt -from 40 -to 80 -own
//	go run ./cmd/chatprobe -log temp/log.txt -backends 'mine|https://api.example.com/v1|some-model|sk-...'
//	go run ./cmd/chatprobe -backends "$CHAT_BACKENDS" -voice-backends g4f-3
//	go run ./cmd/chatprobe -log temp/log.txt -cache temp/answers.json -seed 7
//
// The last form records every model answer and seeds the code's own
// randomness, so running it again repeats the run exactly: what a bug did
// once, it does again.
//
// The last form is how relays are ranked on holding her voice: the same log,
// thinking through the whole list, speaking through one relay at a time.
//
// At every point where she spoke in the log in answer to someone, it asks v2
// what she makes of the moment and what she says, and prints that beside
// what she actually said. Her memory is a scratch directory, printed at the
// end, so what she wrote down can be read as files.
//
// By default the transcript carries on with the lines she really said, so
// every point compares like with like. -own carries on with v2's lines
// instead, which is the fairer test of whether she holds together over a
// whole conversation — the thing v1 failed at — but the people in the log
// were answering someone else by then.
//
// The production log that retired v1 is the regression scenario: v2 has to
// own what she said, soften to an apology, and never post a control word.
// See docs/persona.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/rs/zerolog"
)

// probeGuild is the guild id the scratch memory is kept under.
const probeGuild = "probe"

func main() {
	logPath := flag.String("log", "temp/log.txt", "conversation copied out of Discord")
	card := flag.String("character", "data/character.md", "character file to load")
	botName := flag.String("as", "Server Domme", "her name as it heads her messages in the log")
	todayFlag := flag.String("today", "", `the day the log was copied, YYYY-MM-DD, for "Yesterday at" and bare times; default today`)
	memDir := flag.String("memory", "", "memory directory to use; default a fresh scratch one")
	from := flag.Int("from", 0, "first of her replies to probe, counting from 1")
	to := flag.Int("to", 0, "last of her replies to probe; 0 for all")
	own := flag.Bool("own", false, "carry the conversation on with v2's lines instead of the ones in the log")
	reflect := flag.Bool("reflect", true, "reflect on each day as the log moves past it")
	backends := flag.String("backends", "", "backends as in CHAT_BACKENDS; default the g4f relay")
	selfFacts := flag.Bool("self-facts", true, "read her own words for facts about herself when reflecting, as CHAT_SELF_FACTS")
	examples := flag.Int("examples", 8, "examples her voice sees per message, as CHAT_EXAMPLES_SAMPLE; 0 for all")
	voiceBackends := flag.String("voice-backends", "", "names from -backends her voice prefers, in order, as in CHAT_VOICE_BACKENDS; run one at a time to rank relays on holding her voice")
	cachePath := flag.String("cache", "", "record every model answer to this file, and answer from it when the same prompt comes again: a run can be repeated exactly")
	seed := flag.Uint64("seed", 0, "seed the code's own randomness (example sampling, drift); 0 for random")
	drift := flag.Float64("drift", 0.25, "odds recall brings back a loosely related memory, as CHAT_DRIFT")
	feelings := flag.Bool("feelings", true, "feelings that fade in place of a mood, as CHAT_FEELINGS")
	flag.Parse()

	log := zerolog.New(zerolog.NewConsoleWriter()).Level(zerolog.WarnLevel)
	loc := time.Local

	character, err := mind.LoadCharacter("Domme", *card)
	if err != nil {
		fail(err)
	}

	today := time.Now().In(loc)
	if *todayFlag != "" {
		if today, err = time.ParseInLocation("2006-01-02", *todayFlag, loc); err != nil {
			fail(fmt.Errorf("chatprobe: -today: %w", err))
		}
	}
	f, err := os.Open(*logPath)
	if err != nil {
		fail(err)
	}
	msgs, err := parseLog(f, *botName, today, loc)
	_ = f.Close()
	if err != nil {
		fail(err)
	}

	dir := *memDir
	if dir == "" {
		if dir, err = os.MkdirTemp("", "chatprobe-mind-"); err != nil {
			fail(err)
		}
	}
	store, err := memory.Open(dir, loc)
	if err != nil {
		fail(err)
	}

	opts := ai.Options{UseG4F: true, G4FPicks: 3}
	if *backends != "" {
		opts = ai.Options{Extra: strings.Split(*backends, ",")}
	}
	if *voiceBackends != "" {
		opts.Voice = strings.Split(*voiceBackends, ",")
	}
	pool, err := ai.Build(context.Background(), log, opts)
	if err != nil {
		fail(err)
	}
	var thinking, voice ai.Provider = pool, pool.Voice()
	var cache *cacheFile
	if *cachePath != "" {
		if cache, err = openCache(*cachePath); err != nil {
			fail(err)
		}
		thinking, voice = cache.wrap(pool, false), cache.wrap(pool.Voice(), true)
	}
	var roll func() float64
	if *seed != 0 {
		roll = rand.New(rand.NewPCG(*seed, *seed)).Float64
	}

	p := &probe{
		mind: &mind.Mind{
			Character: character, Provider: thinking, Voice: voice, Memory: store,
			SelfFacts: *selfFacts, ExamplesSample: *examples, Drift: *drift, Feelings: *feelings,
			Roll: roll,
		},
		botName: *botName,
		own:     *own,
		reflect: *reflect,
		from:    *from,
		to:      *to,
	}
	fmt.Printf("character: %s · %d messages in the log · memory in %s\n\n", character.Name, len(msgs), dir)
	p.replay(msgs)
	if cache != nil {
		if err := cache.save(); err != nil {
			fail(err)
		}
		fmt.Printf("\nmodel answers recorded in %s\n", *cachePath)
	}
	fmt.Printf("\nher memory is in %s\n", dir)
}

// probe is one replay in progress.
type probe struct {
	mind    *mind.Mind
	botName string
	own     bool
	reflect bool
	from    int
	to      int

	turns   []mind.Turn
	replies int
	said    int
	day     time.Time
}

func (p *probe) replay(msgs []message) {
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		p.dayTurn(m.At)
		if !m.Bot {
			p.turns = append(p.turns, mind.Turn{UserID: idOf(m.Author), Username: m.Author, Content: m.Text, At: m.At})
			continue
		}

		// Her lines that follow one another are one reply sent as several.
		said := []string{m.Text}
		for i+1 < len(msgs) && msgs[i+1].Bot && msgs[i+1].At.Sub(m.At) < 2*time.Minute {
			i++
			said = append(said, msgs[i].Text)
		}
		original := strings.Join(said, " / ")

		asker, trigger := p.approach()
		if asker == nil {
			// She spoke unprompted in the log. Replayed as what she said, so
			// the rest of the conversation has it, and not probed: what she
			// would start is a different question from what she would answer.
			fmt.Printf("── %s  (she started this herself)\n   v1: %s\n\n", m.At.Format("02.01 15:04"), original)
			p.recordHers(original, m.At, "", mind.TriggerStart)
			continue
		}

		p.replies++
		if p.replies < p.from || (p.to > 0 && p.replies > p.to) {
			p.recordHers(original, m.At, asker.UserID, trigger)
			continue
		}
		mine := p.answer(*asker, trigger, m.At, original)
		if p.own && mine != "" {
			p.recordHers(mine, m.At, asker.UserID, trigger)
		} else {
			p.recordHers(original, m.At, asker.UserID, trigger)
		}
	}
	if p.reflect && !p.day.IsZero() {
		p.reflectOn(p.day, p.day.Add(30*time.Hour))
	}
}

// approach is who she is answering at this point, and how they reached her:
// the last person to speak since she did, and a mention if they tagged her.
func (p *probe) approach() (*mind.Turn, mind.Trigger) {
	for i := len(p.turns) - 1; i >= 0; i-- {
		t := p.turns[i]
		if t.FromBot {
			return nil, ""
		}
		if strings.Contains(strings.ToLower(t.Content), "@"+strings.ToLower(p.botName)) {
			return &t, mind.TriggerMention
		}
		if i == 0 || p.turns[i-1].FromBot {
			trigger := mind.TriggerFollowUp
			if i == 0 || t.At.Sub(p.turns[i-1].At) > 3*time.Minute {
				trigger = mind.TriggerNamed
			}
			last := p.turns[len(p.turns)-1]
			return &last, trigger
		}
	}
	return nil, ""
}

// answer runs one moment through v2 and prints it beside what v1 said.
func (p *probe) answer(asker mind.Turn, trigger mind.Trigger, at time.Time, original string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	sc := mind.Scene{
		GuildID: probeGuild, GuildName: "probe", ChannelName: "chat",
		SelfName: p.botName, Now: at, Turns: p.live(at),
		Trigger: trigger, UserID: asker.UserID, Username: asker.Username,
	}
	fmt.Printf("── #%d %s  %s → %s\n", p.replies, at.Format("02.01 15:04"), asker.Username, trigger)
	for _, t := range tail(sc.Turns, 3) {
		who := t.Username
		if t.FromBot {
			who = "her"
		}
		fmt.Printf("   %s: %s\n", who, t.Content)
	}

	known, err := p.mind.Know(sc)
	if err != nil {
		fmt.Printf("   ! memory: %v\n", err)
	}
	a, err := p.mind.Consider(ctx, sc, known)
	if err != nil {
		fmt.Printf("   ! consider: %v\n   v1: %s\n\n", err, original)
		return ""
	}
	if err := p.mind.Absorb(sc, a); err != nil {
		fmt.Printf("   ! absorb: %v\n", err)
	}
	fmt.Printf("   read: %s\n   feel: %s · toward them: %s · mood: %s\n", a.Read, a.Feel, a.Toward, a.Mood)
	if a.Note != "" || a.Between != "" || a.Later != "" {
		fmt.Printf("   noted: %s · between: %s · later: %s\n", a.Note, a.Between, a.Later)
	}
	fmt.Printf("   v1: %s\n", original)

	switch a.Act {
	case mind.ActIgnore:
		fmt.Printf("   v2: (lets it go)  [%s]\n\n", a.Backend)
		_ = p.mind.LetGo(sc, a)
		return ""
	case mind.ActReact:
		fmt.Printf("   v2: (reacts %s)  [%s]\n\n", a.Emoji, a.Backend)
		_ = p.mind.LetGo(sc, a)
		return ""
	}
	reply, backend, err := p.mind.Speak(ctx, sc, known, a, "")
	if err != nil {
		fmt.Printf("   v2: ! %v  (meant: %s)\n\n", err, a.Intent)
		return ""
	}
	reply = mind.Casual(reply, mind.CasualStyle{}, 1)
	fmt.Printf("   v2: %s\n   -- meant: %s  [%s]\n\n", reply, a.Intent, backend)
	if err := p.mind.Said(sc, a, reply, "", p.nextID()); err != nil {
		fmt.Printf("   ! said: %v\n", err)
	}
	return reply
}

// recordHers puts a line of hers into the transcript, and into her memory
// when it is not one v2 already recorded.
func (p *probe) recordHers(text string, at time.Time, to string, trigger mind.Trigger) {
	p.turns = append(p.turns, mind.Turn{FromBot: true, Content: text, At: at, To: to})
	if p.own && trigger != mind.TriggerStart {
		return
	}
	sc := mind.Scene{GuildID: probeGuild, ChannelName: "chat", Now: at, Trigger: trigger, UserID: to, Turns: p.turns[:len(p.turns)-1]}
	for _, t := range p.turns {
		if t.UserID == to && t.Username != "" {
			sc.Username = t.Username
		}
	}
	_ = p.mind.Said(sc, mind.Appraisal{}, text, "", p.nextID())
}

// nextID is a message id for a line of hers. The log carries none, and
// self-facts cite her lines by id, so the replay makes them up.
func (p *probe) nextID() string {
	p.said++
	return fmt.Sprintf("probe%d", p.said)
}

// live is the conversation as the bot's buffer would hold it: the last half
// hour, or at least the last eight lines, and nothing older than a day. See
// mind.Conversations.
func (p *probe) live(now time.Time) []mind.Turn {
	start := len(p.turns)
	for i := len(p.turns) - 1; i >= 0; i-- {
		age := now.Sub(p.turns[i].At)
		if age > mind.MaxTurnAge || (age > mind.TurnStaleAfter && len(p.turns)-i > mind.MinLiveTurns) {
			break
		}
		start = i
	}
	return append([]mind.Turn(nil), p.turns[start:]...)
}

// dayTurn reflects on the day just finished when the log moves into a new
// one, the way the running bot does at night.
func (p *probe) dayTurn(at time.Time) {
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
	if !p.day.IsZero() && day.After(p.day) && p.reflect {
		p.reflectOn(p.day, day.Add(5*time.Hour))
	}
	p.day = day
}

func (p *probe) reflectOn(day, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	did, err := p.mind.Reflect(ctx, probeGuild, "probe", day, now, nil)
	switch {
	case err != nil:
		fmt.Printf("══ reflecting on %s failed: %v\n\n", day.Format("02.01"), err)
	case did:
		d, _ := p.mind.Memory.Day(probeGuild, day)
		me, _ := p.mind.Memory.Self(probeGuild)
		fmt.Printf("══ she looks back on %s:\n   %s\n   lately: %s\n\n", day.Format("02.01"), d.Summary, me.Lately)
	}
	if !p.mind.SelfFacts {
		return
	}
	did, err = p.mind.ReflectSelf(ctx, probeGuild, day)
	switch {
	case err != nil:
		fmt.Printf("══ reading her own words on %s failed: %v\n\n", day.Format("02.01"), err)
	case did:
		me, _ := p.mind.Memory.Me(probeGuild)
		fmt.Printf("══ what she has said about herself, as of %s:\n", day.Format("02.01"))
		for _, f := range me.Facts {
			line := "   - " + f.Text
			if f.Conflict != "" {
				line += "   (against the card: " + f.Conflict + ")"
			}
			fmt.Println(line)
		}
		fmt.Println()
	}
}

func tail(turns []mind.Turn, n int) []mind.Turn {
	if len(turns) > n {
		return turns[len(turns)-n:]
	}
	return turns
}

// idOf gives a name from the log a stable id, since a copied log has none.
func idOf(name string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(name)))
	return fmt.Sprintf("u%d", h.Sum32())
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
