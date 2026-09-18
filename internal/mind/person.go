package mind

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
)

// What she knows about one person, beyond counting their messages.
//
// Three things, each for a different reason. Facts are what someone said
// about themselves, so she can call back to them the way anyone who was
// listening would. The impression is her own opinion of them, so two members
// who both count as "a regular" are not interchangeable to her. Warmth is the
// counterpart of irritation: something that builds when talking to someone
// goes well, and changes how she treats them rather than what she knows.
//
// The design keeps cognitum's two good ideas here and avoids the two ways it
// went wrong. Facts there were kept first-come forever, so a job change was
// never learned; here the newest value for a key wins. Its reflection ran
// every twenty seconds over the same memories and talked itself into a
// fixation; here the impression is revised at most once per remembered
// conversation, starting from the previous one rather than from nothing.
const (
	// MaxFacts is how many facts she keeps per person. The oldest go first.
	MaxFacts = 12
	// renderedFacts is how many reach a prompt at once, newest first. The
	// grounding budget is shared with memories and the room, and a list of
	// twelve facts about one person reads as a dossier rather than as
	// knowing someone.
	renderedFacts = 4

	maxFactKey     = 24
	maxFactValue   = 80
	maxImpression  = 160
	maxNotedPeople = 5

	// WarmthHalflife is how long a fondness takes to halve without contact.
	// Far slower than irritation: being annoyed with someone passes in an
	// afternoon, liking them does not.
	WarmthHalflife = 21 * 24 * time.Hour
	warmthFloor    = 0.05

	fondAbove  = 0.6
	likesAbove = 0.3
)

// Fact is one thing a person said about themselves.
type Fact struct {
	Key   string
	Value string
	At    time.Time
}

// ClosenessNow decays a stored closeness to the present, the same way tension
// is: stored once with its time, never swept.
func ClosenessNow(stored float64, at, now time.Time) float64 {
	if stored <= 0 || at.IsZero() {
		return 0
	}
	elapsed := now.Sub(at)
	if elapsed <= 0 {
		return clamp01(stored)
	}
	level := stored * math.Pow(0.5, float64(elapsed)/float64(WarmthHalflife))
	if level < warmthFloor {
		return 0
	}
	return clamp01(level)
}

// WarmthDirective is the instruction for someone she has come to like, or ""
// when she has not. Phrased as something she would not admit to, because
// open affection from this character would be a different character.
func WarmthDirective(name string, level float64) string {
	if name == "" {
		return ""
	}
	switch {
	case level > fondAbove:
		return fmt.Sprintf(
			"You are fond of %s, not that you would say so. Let it show in small "+
				"ways: more patience, a sharper tease, the benefit of the doubt.", name)
	case level > likesAbove:
		return fmt.Sprintf("You like %s well enough. Give them a little more than you give most.", name)
	default:
		return ""
	}
}

// MergeFacts folds newly learned facts into what she already knew.
//
// A key she already has takes the new value: people change jobs and move
// cities, and a first answer kept forever is how she ends up confidently
// wrong about someone. Past MaxFacts the oldest go.
func MergeFacts(known, learned []Fact) []Fact {
	byKey := make(map[string]Fact, len(known)+len(learned))
	for _, f := range known {
		byKey[f.Key] = f
	}
	for _, f := range learned {
		if f.Key == "" || f.Value == "" {
			continue
		}
		byKey[f.Key] = f
	}

	out := make([]Fact, 0, len(byKey))
	for _, f := range byKey {
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > MaxFacts {
		out = out[:MaxFacts]
	}
	return out
}

// PersonNote is what she already has on someone, given to the notes call so
// it revises rather than starts over.
type PersonNote struct {
	Name       string
	Impression string
	Facts      []Fact
}

// PersonUpdate is what the notes call returned for one person.
type PersonUpdate struct {
	Name       string
	Facts      []Fact
	Plans      []Plan
	Impression string
}

// notesInstruction asks for the person file in a form that survives a weak
// model: two line shapes and a word for nothing.
//
// The exclusions are not squeamishness. Everything here is kept indefinitely
// and repeated back into later conversations, possibly in front of other
// people; a character who casually brings up someone's diagnosis or address
// in a public channel does real harm, whatever the server is about.
const notesInstruction = `You keep private notes on the people in a Discord server. You are "you" in the conversation below. Update your notes on the other people in it.

Write only lines in these three forms, and nothing else:
FACT <name>: <key> = <value>
PLAN <name>: <what> | <when>
IMPRESSION <name>: <one sentence>

FACT is only for something a person plainly said about themselves: their job, their city, a pet, what they are into. Never guess or infer, and never note what someone said about somebody else. The key is one or two words (job, city, pet, hobby); the value is a few words.

PLAN is only for something a person said they themselves are about to do: an interview, a trip, an exam, a match, a date. Not someone else's plans, and not a habit. <what> is a few words in their terms; <when> is exactly one of: tonight, tomorrow, this weekend, next week, later.
Never note health, sexuality, religion, politics, real names, addresses, contact details, or anything else a person would not want repeated in public.

IMPRESSION is your own private opinion of the person, in your own voice and as bluntly as you would think it, one short sentence. If you already have one, it is listed below: keep what still holds, and change it only as far as this conversation gives you reason to. Only for people who said enough to judge.

If there is nothing worth noting, write NONE.`

// NotesPrompt builds the call that updates the person file after a
// conversation has been remembered.
//
// The persona goes in because the impression is rendered back into her
// prompt as "your take". Written without it, the relays produced a neutral
// caseworker's note — "seems friendly and observant" — which then sat in her
// prompt pulling her voice towards the same register.
func NotesPrompt(persona string, turns []Turn, known []PersonNote) []ai.Message {
	var b strings.Builder
	if len(known) > 0 {
		b.WriteString("Your notes so far:\n")
		for _, p := range known {
			if p.Impression == "" && len(p.Facts) == 0 {
				continue
			}
			fmt.Fprintf(&b, "- %s:", p.Name)
			if p.Impression != "" {
				fmt.Fprintf(&b, " %s", p.Impression)
			}
			for _, f := range p.Facts {
				fmt.Fprintf(&b, " [%s = %s]", f.Key, f.Value)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("The conversation:\n")

	var transcript strings.Builder
	for _, t := range turns {
		if strings.TrimSpace(t.Content) == "" {
			continue
		}
		who := t.Username
		if t.FromBot {
			who = "you"
		}
		if who == "" {
			who = "someone"
		}
		transcript.WriteString(who + ": " + t.Content + "\n")
	}
	b.WriteString(trimTo(transcript.String(), summaryTranscriptChars))

	system := notesInstruction
	if p := strings.TrimSpace(persona); p != "" {
		system = "Who you are:\n" + p + "\n\n" + notesInstruction
	}
	return []ai.Message{
		{Role: ai.RoleSystem, Content: system},
		{Role: ai.RoleUser, Content: b.String()},
	}
}

// ParseNotes reads the notes call's reply.
//
// Lenient about everything a relay varies — case, bullets, bold markers, a
// stray preamble — and strict about the shape of each line, because a line
// misread as a fact is stored about a real person and repeated back to them.
// Unknown lines are ignored rather than failing the whole reply.
func ParseNotes(reply string, at time.Time) []PersonUpdate {
	byName := make(map[string]*PersonUpdate)
	var order []string
	get := func(name string) *PersonUpdate {
		key := strings.ToLower(name)
		if u, ok := byName[key]; ok {
			return u
		}
		u := &PersonUpdate{Name: name}
		byName[key] = u
		order = append(order, key)
		return u
	}

	for _, raw := range strings.Split(reply, "\n") {
		line := strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), "-*•"))
		line = strings.ReplaceAll(line, "**", "")
		kind, rest, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		name, body, ok := strings.Cut(rest, ":")
		name = strings.TrimSpace(name)
		body = strings.TrimSpace(body)
		if !ok || name == "" || body == "" || len(name) > 40 {
			continue
		}

		switch strings.ToUpper(kind) {
		case "FACT":
			key, value, ok := strings.Cut(body, "=")
			key = FactKey(key)
			value = clipRunes(strings.TrimSpace(value), maxFactValue)
			if !ok || key == "" || value == "" {
				continue
			}
			u := get(name)
			u.Facts = append(u.Facts, Fact{Key: key, Value: value, At: at})
		case "PLAN":
			what, when, ok := strings.Cut(body, "|")
			what = strings.TrimSpace(what)
			if !ok || what == "" {
				continue
			}
			u := get(name)
			u.Plans = append(u.Plans, Plan{What: what, When: strings.TrimSpace(when)})
		case "IMPRESSION":
			get(name).Impression = clipRunes(body, maxImpression)
		}
	}

	out := make([]PersonUpdate, 0, len(order))
	for _, key := range order {
		out = append(out, *byName[key])
		if len(out) == maxNotedPeople {
			break
		}
	}
	return out
}

// FactKey normalises a fact's key, so "Job", "job " and "the job" are one
// fact and a new value replaces the old one.
func FactKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.TrimPrefix(key, "the ")
	key = strings.TrimPrefix(key, "their ")
	key = strings.Join(strings.Fields(key), "_")
	return clipRunes(key, maxFactKey)
}

func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max]))
}

// renderKnown is what she knows about someone, for the people block: the
// newest few facts and her impression.
func renderKnown(facts []Fact, impression string) string {
	var parts []string
	if n := len(facts); n > 0 {
		if n > renderedFacts {
			facts = facts[:renderedFacts]
		}
		items := make([]string, 0, len(facts))
		for _, f := range facts {
			items = append(items, strings.ReplaceAll(f.Key, "_", " ")+": "+f.Value)
		}
		parts = append(parts, "you know "+strings.Join(items, "; "))
	}
	if impression != "" {
		parts = append(parts, "your take: "+impression)
	}
	return strings.Join(parts, ". ")
}
