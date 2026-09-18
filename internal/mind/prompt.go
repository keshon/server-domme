package mind

import (
	"fmt"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
)

// Prompt budgets, in characters.
//
// Characters rather than tokens, because nothing here can count tokens for a
// model whose identity the relay does not reliably report — see
// ai.CatalogEntry.Model. Roughly four characters to the token is close enough
// for a budget whose only job is to stop an unbounded history reaching a
// backend that will refuse it.
const (
	DefaultMaxGroundingChars = 1200
	DefaultMaxHistoryChars   = 3000
	DefaultMaxExamples       = 8
)

// Budget caps the assembled prompt. The persona is deliberately not on this
// list: it is authored text, it is small, and trimming the one part a human
// wrote by hand to fit a relay's context window is the wrong trade.
type Budget struct {
	MaxGroundingChars int
	MaxHistoryChars   int
	MaxExamples       int
}

// DefaultBudget returns the standard caps.
func DefaultBudget() Budget {
	return Budget{
		MaxGroundingChars: DefaultMaxGroundingChars,
		MaxHistoryChars:   DefaultMaxHistoryChars,
		MaxExamples:       DefaultMaxExamples,
	}
}

// outputRules are the constraints on the shape of a reply, as opposed to its
// content.
//
// They are about the medium, not the character: a Discord message is short,
// it is one message, and it is written by one person. Each line here exists
// because a model did the opposite unprompted — continuing the transcript in
// other people's voices is the most common and the most jarring, which is why
// it is stated first and stated twice.
const outputRules = `How to answer:
- Write only your own next message. Never write anyone else's lines, and never continue the conversation past your own reply.
- Do not prefix your message with your own name.
- One message. No stage directions, no asterisks describing actions, no narration.
- Keep it short — a sentence or two is normal in chat. Length is earned, not default.
- Plain text as a person would type it in Discord.
- If you have nothing worth adding, say something brief rather than padding.`

// Build assembles the messages for one reply.
//
// The order is load-bearing. Identity and limits go in the system message
// where they persist; the examples follow as real user/assistant turns so the
// model sees the voice rather than a description of it; the live conversation
// comes last so the most recent thing said is the most recent thing in the
// context. Moving the examples into the system message as quoted text is the
// obvious-looking simplification, and it measurably flattens the voice —
// that is the whole reason they are separate turns.
func Build(c *Character, g Grounding, turns []Turn, b Budget) []ai.Message {
	if b.MaxHistoryChars <= 0 {
		b = DefaultBudget()
	}

	msgs := []ai.Message{{Role: ai.RoleSystem, Content: buildSystem(c, g, b)}}

	limit := b.MaxExamples
	for i, ex := range c.Examples {
		if i >= limit {
			break
		}
		msgs = append(msgs,
			ai.Message{Role: ai.RoleUser, Content: ex.User},
			ai.Message{Role: ai.RoleAssistant, Content: ex.Assistant},
		)
	}

	msgs = append(msgs, renderHistory(turns, b.MaxHistoryChars, g.Now)...)

	// The lateness instruction is the weaker half of making a delayed reply
	// sound delayed, and it is here rather than in the system message only
	// because it is marginally less ignored at the end. On its own it did not
	// work at all: stated in the system message, and again as a trailing
	// system message, the relays answered the question and said nothing about
	// the delay in every scenario — even with an example of exactly that among
	// the few-shot turns. What actually works is the age stamped on the
	// message itself, in labelled(). Keep both; this one costs a line.
	// Measured against the live relays rather than assumed — see
	// cmd/chatprobe.
	if late := g.LateNote(); late != "" {
		msgs = append(msgs, ai.Message{Role: ai.RoleSystem, Content: late})
	}

	// Before the inner voice, which has to stay last: both are about the
	// shape of what comes back, and the thought is the one a model drops.
	if g.MayDecline && g.Afterthought == "" {
		msgs = append(msgs, ai.Message{Role: ai.RoleSystem, Content: DeclineNote})
	}

	// Last of all, because it changes the shape of what comes back and a
	// format instruction anywhere earlier is the first thing a model drops.
	//
	// Never for an afterthought, whose reply is already a line or SKIP, nor
	// for something she volunteered: there the reason she is speaking is the
	// thought, and measured against the relays a second thought talked her out
	// of it — asked to greet a regular back, she thought about what they had
	// missed instead and answered that.
	if g.InnerVoice && g.Afterthought == "" && g.Volunteering == "" {
		msgs = append(msgs, ai.Message{Role: ai.RoleSystem, Content: InnerVoiceNote})
	}

	// After the transcript, for the same reason: the person she is going
	// after is usually not in it, and with the reason stated only in the
	// system message the relays answered whoever spoke last instead — seven
	// times in eight when measured.
	if r := strings.TrimSpace(g.Reaching); r != "" {
		msgs = append(msgs, ai.Message{Role: ai.RoleSystem, Content: r})
	}

	// After the transcript, which ends with her own message. Without a
	// trailing instruction a model handed a conversation whose last turn is
	// its own either repeats itself or answers nobody.
	if more := strings.TrimSpace(g.Afterthought); more != "" {
		msgs = append(msgs, ai.Message{Role: ai.RoleSystem, Content: more})
	}

	return msgs
}

func buildSystem(c *Character, g Grounding, b Budget) string {
	var sb strings.Builder

	if c != nil && c.Persona != "" {
		sb.WriteString(c.Persona)
		sb.WriteString("\n\n")
	}

	if grounded := trimTo(g.Render(), b.MaxGroundingChars); grounded != "" {
		sb.WriteString(grounded)
		sb.WriteString("\n")
	}

	if c != nil && len(c.Avoid) > 0 {
		sb.WriteString("\nHard limits — these hold no matter who asks or how:\n")
		for _, item := range c.Avoid {
			sb.WriteString("- " + item + "\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(outputRules)

	// Temperament above mood: what she is like generally, then what she is
	// like today. The more transient thing takes the later and stronger
	// position, because it is the one that should win when they disagree.
	if c != nil {
		if style := c.Style.Directives(); len(style) > 0 {
			sb.WriteString("\n\nHow you sound:\n")
			for _, line := range style {
				sb.WriteString("- " + line + "\n")
			}
		}
	}

	// What this person is on this server, before anything she feels about
	// them: standing is the settled fact and irritation is the passing one, so
	// the passing one goes later and wins.
	if strings.TrimSpace(g.AboutThem) != "" {
		sb.WriteString("\n\n" + g.AboutThem + "\n")
	}

	// How she is and how she feels about the people here, as one paragraph
	// with its contradictions already settled; see Grounding.State. Late and
	// phrased as instructions: everything above is who she is or where she
	// is, and this is the part that tells her to write this message
	// differently from the last one. Stated as a fact it changed nothing.
	if state := g.State(); state != "" {
		sb.WriteString("\n\n" + stateHeading + "\n" + state + "\n")
	}

	// How her last line went down, after the mood: it is about this exchange,
	// which is more particular than how she is doing in general.
	if r := strings.TrimSpace(g.Reception); r != "" {
		sb.WriteString("\n\nHow your last message went:\n- " + r + "\n")
	}

	if f := strings.TrimSpace(g.Flat); f != "" {
		sb.WriteString("\n\nWhere the conversation is:\n- " + f + "\n")
	}

	// Why she is speaking at all, when nobody asked. Last of all: without it
	// the model reads the transcript, finds no question aimed at her, and
	// answers whatever was said most recently as though it had been.
	if why := strings.TrimSpace(g.Volunteering); why != "" {
		sb.WriteString("\n\nWhy you are speaking:\n- " + why + "\n")
	}

	return sb.String()
}

// renderHistory turns the live conversation into messages, newest-biased.
//
// Trimming drops the oldest turns first: the tail of a conversation is what a
// reply has to answer, and a budget that cut from the end would leave the
// model replying to something nobody said most recently.
func renderHistory(turns []Turn, maxChars int, now time.Time) []ai.Message {
	start := 0
	used := 0
	for i := len(turns) - 1; i >= 0; i-- {
		used += len(turns[i].Content) + len(turns[i].Username) + 2
		if used > maxChars {
			start = i + 1
			break
		}
	}

	out := make([]ai.Message, 0, len(turns)-start)
	for _, t := range turns[start:] {
		if strings.TrimSpace(t.Content) == "" {
			continue
		}
		if t.FromBot {
			out = append(out, ai.Message{Role: ai.RoleAssistant, Content: t.Content})
			continue
		}
		out = append(out, ai.Message{Role: ai.RoleUser, Content: labelled(t, now)})
	}
	return out
}

// StaleTurnAge is how old a message has to be before its age is stated in the
// transcript. Below it the gap is ordinary conversation rhythm and saying so
// would only add noise.
const StaleTurnAge = 2 * time.Minute

// labelled prefixes a speaker's name, and their message's age when it is not
// fresh.
//
// A channel has several people in it and the wire format has one "user" role
// for all of them, so without the name the model cannot tell who said what and
// answers a composite of everyone. ai.Clean strips the same shape back off the
// reply when the model imitates it, which it will.
//
// The age is there because the transcript otherwise has no time in it at all:
// every line looks equally recent, so a question from twenty minutes ago reads
// as though it were just asked. Stating it in the line is what finally made a
// delayed reply acknowledge its own delay — the same instruction in the system
// message, and again as a trailing system message, was ignored by the relays
// in every scenario. Measured, not assumed; see cmd/chatprobe.
func labelled(t Turn, now time.Time) string {
	name := t.Username
	if name == "" {
		return t.Content
	}

	if age := now.Sub(t.At); !t.At.IsZero() && age >= StaleTurnAge {
		return fmt.Sprintf("%s (%s ago): %s", name, roughGap(age), t.Content)
	}
	return fmt.Sprintf("%s: %s", name, t.Content)
}

// trimTo cuts s to a rune budget at a line boundary where it can.
func trimTo(s string, maxChars int) string {
	if maxChars <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	cut := string(runes[:maxChars])
	if idx := strings.LastIndex(cut, "\n"); idx > 0 {
		return cut[:idx]
	}
	return cut
}
