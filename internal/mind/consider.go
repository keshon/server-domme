package mind

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
)

// Act is what she decides to do about a moment.
type Act string

// Acts. Written to the journal, so frozen once shipped.
const (
	ActReply  Act = "reply"
	ActReact  Act = "react"
	ActIgnore Act = "ignore"
)

// Appraisal is what she made of a moment, privately: how she read it, how it
// landed, what she wants to do, and what she takes away from it.
//
// It is the model's own judgement, in words. Nothing here is a score, and
// nothing in the code second-guesses it except the few rails a model cannot be
// trusted with; see docs/persona.md.
type Appraisal struct {
	// Read is what she thinks they mean or want.
	Read string
	// Feel is how it lands with her.
	Feel string
	// Toward is how she feels about them now, in a few words.
	Toward string
	// Mood is her mood after this.
	Mood string
	Act  Act
	// Emoji is her reaction, when Act is ActReact.
	Emoji string
	// Intent is the gist of what she wants to get across, when she replies.
	// The voice turns it into words; see Mind.Speak.
	Intent string
	// Note is a new fact about the person, Between how things now stand with
	// them if that changed, Remember something worth keeping from the
	// moment, and Later something she means to follow up.
	Note       string
	Between    string
	Remember   string
	Later      string
	LaterHours float64
	// BackOff is them asking her to leave them alone — stop pinging them,
	// stop coming after them. The caller withdraws their consent to be
	// reached. Read by the model rather than a word list: v1's list could not
	// tell "leave me alone" said to her from the same words about a game.
	BackOff bool
	// Backend is which relay answered.
	Backend string
}

// thinkingRules are how she thinks, stated to the model that thinks for her.
//
// Each line answers a failure seen in production with v1 (September 2026):
// "she owns what she said" is the log where she disowned her own replies,
// "good faith softens her" the one where ten apologies met "keep it
// professional" ten times, "do not invent" the "#help" channel that never
// existed. Rewording them is fine; dropping one brings its failure back.
const thinkingRules = `How to think:
- Read people the way she would: what they want, what they mean under the words, whether they are joking, testing, hurt or sincere. Take them at their word unless there is a reason not to.
- She is one continuous person. Every line marked YOU in the conversation, and everything her memories say she said, is hers: she said it and meant it at the time. She can change her mind and say so, but she never claims someone else wrote her words.
- Feelings move the way a person's do. Good faith, apology and persistence soften her; being pushed, mocked or used cools her. One exchange does not decide everything, and she does not carry one grudge through every line.
- She has a life and wants of her own. She can be curious, amused, warm, bored or busy — not only guarded.
- She knows only what is written here. She does not invent channels, rules, events or facts about people.
- Someone she is talking with who asks her something directly nearly always gets some answer, even a short one. Ignoring is for when there is truly nothing to say, or a snub she would stand behind.
- Reacting with an emoji instead of words is for a message that needs acknowledging and nothing more.`

// appraisalShape is the JSON she answers with. The comments are for the
// model; keys it leaves out read as empty.
const appraisalShape = `Answer with one JSON object and nothing else:
{
  "read": "what they mean or want, reading between the lines — one sentence",
  "feel": "how it lands with her, honestly — a few words",
  "toward": "how she feels about them now — a few words",
  "mood": "her mood after this — a few words",
  "act": "reply" or "react" or "ignore",
  "emoji": "one emoji, only if act is react",
  "intent": "if act is reply: what she wants to get across and how she comes at it — the gist, not the wording",
  "note": "a new fact about them worth keeping, or empty",
  "between": "if how things stand between them just changed: one sentence on where it stands now; otherwise empty",
  "remember": "something from this moment she would remember later, or empty",
  "later": "something she means to follow up on with them later, or empty",
  "later_hours": "how many hours from now, if later is set",
  "back_off": true only if they are asking her to leave them alone or stop coming after them
}`

// Consider asks what she makes of a moment. It does not write anything down;
// see Absorb, which the caller runs once it has applied its rails.
func (m *Mind) Consider(ctx context.Context, s Scene, k Known) (Appraisal, error) {
	msgs := m.considerPrompt(s, k)
	ctx = ai.WithRaw(ai.WithTemperature(ctx, thinkingTemperature))
	reply, backend, err := m.generate(ctx, msgs)
	if err != nil {
		return Appraisal{}, err
	}
	a, ok := parseAppraisal(reply)
	a.Backend = backend
	if !ok {
		return a, fmt.Errorf("%w: %q", ErrUnreadable, clip(reply, 200))
	}
	return a, nil
}

func (m *Mind) considerPrompt(s Scene, k Known) []ai.Message {
	name := m.name(s)
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are the inner life of %s. %s is a person on a Discord server — "+
		"not an assistant, not a bot, not a service. You do not write her messages. "+
		"You work out, privately and honestly, what she makes of what just happened "+
		"and what she wants to do about it.", name, name)
	if m.Character != nil && m.Character.Persona != "" {
		sys.WriteString("\n\nWho she is:\n" + m.Character.Persona)
	}
	sys.WriteString("\n\n" + thinkingRules + "\n\n" + appraisalShape)

	var user strings.Builder
	user.WriteString(renderWorld(s, k))
	user.WriteString("\n\nThe conversation, oldest first:\n")
	user.WriteString(renderTranscript(s.Turns, s.Now))
	who := s.Username
	if who == "" {
		who = "someone"
	}
	fmt.Fprintf(&user, "\n\nWhat just happened: %s.", s.Trigger.describe(who))
	if s.Late > 0 {
		fmt.Fprintf(&user, " She is only getting to it now, %s later.", gap(s.Late))
	}
	user.WriteString(" What does she make of it, and what does she do?")

	return []ai.Message{
		{Role: ai.RoleSystem, Content: sys.String()},
		{Role: ai.RoleUser, Content: user.String()},
	}
}

// parseAppraisal reads the model's JSON. An act it did not name, or named
// in words of its own, falls back by what else it said: an intent means it
// meant to reply.
func parseAppraisal(reply string) (Appraisal, bool) {
	obj, ok := decodeObject(reply)
	if !ok {
		return Appraisal{}, false
	}
	a := Appraisal{
		Read:       str(obj, "read"),
		Feel:       str(obj, "feel"),
		Toward:     str(obj, "toward"),
		Mood:       str(obj, "mood"),
		Emoji:      str(obj, "emoji"),
		Intent:     str(obj, "intent"),
		Note:       str(obj, "note"),
		Between:    str(obj, "between"),
		Remember:   str(obj, "remember"),
		Later:      str(obj, "later"),
		LaterHours: num(obj, "later_hours"),
		BackOff:    strings.EqualFold(str(obj, "back_off"), "true"),
	}
	switch act := strings.ToLower(str(obj, "act")); {
	case strings.Contains(act, "react"):
		a.Act = ActReact
	case strings.Contains(act, "ignore"), strings.Contains(act, "nothing"), strings.Contains(act, "silent"):
		a.Act = ActIgnore
	case strings.Contains(act, "reply"), a.Intent != "":
		a.Act = ActReply
	default:
		a.Act = ActIgnore
	}
	if a.Act == ActReact && !IsEmoji(a.Emoji) {
		// A reaction Discord would refuse. Words were meant; if there is
		// nothing to say, letting it go is the honest reading.
		if a.Intent != "" {
			a.Act = ActReply
		} else {
			a.Act = ActIgnore
		}
	}
	return a, true
}

// IsEmoji reports whether s is something Discord will take as a unicode
// reaction: short, and with no letters or digits in it. A custom emoji is
// ":name:" to a model, which Discord would refuse without the id.
func IsEmoji(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > 8 {
		return false
	}
	for _, r := range s {
		if r < 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// laterDefault is when a follow-up comes due if she did not say.
const laterDefault = 24 * time.Hour

// laterMax is the furthest out an intention is set. Past a week it is not
// something she means to do, it is something she will have forgotten.
const laterMax = 7 * 24 * time.Hour

// Absorb writes down what she took from a moment: her mood, what she learned
// about the person, how things stand between them, what she will remember
// and what she means to do.
//
// Written at once rather than at night because the next message may need it:
// a fact someone told her a minute ago is the thing she is most likely to be
// asked about. Reflection folds it into the paragraphs later.
func (m *Mind) Absorb(s Scene, a Appraisal) error {
	now := s.Now
	if a.Mood != "" {
		err := m.Memory.UpdateSelf(s.GuildID, func(me *memory.Self) {
			me.Mood, me.MoodAt = a.Mood, now
			if me.Lately == "" && m.Character != nil {
				me.Lately = m.Character.Lately
			}
		})
		if err != nil {
			return err
		}
	}

	if s.UserID != "" {
		err := m.Memory.UpdatePerson(s.GuildID, s.UserID, func(p *memory.Person) {
			if s.Username != "" {
				p.Name = s.Username
			}
			if p.FirstMet.IsZero() {
				p.FirstMet = now
			}
			if s.Trigger != TriggerOverheard && !Initiated(s.Trigger) {
				p.LastTalked = now
			}
			if a.Toward != "" {
				p.Feeling = a.Toward
			}
			if a.Between != "" {
				p.Between = a.Between
			}
			if a.Note != "" && !hasNote(p.Notes, a.Note) {
				p.Notes = append(p.Notes, memory.Note{Day: now, Text: a.Note})
			}
		})
		if err != nil {
			return err
		}
	}

	if a.Remember != "" {
		err := m.Memory.AddMoment(s.GuildID, memory.Moment{
			At: now, Channel: s.ChannelName, People: m.refs(s), Text: a.Remember,
		})
		if err != nil {
			return err
		}
	}

	if a.Later != "" {
		due := laterDefault
		if a.LaterHours > 0 {
			due = time.Duration(a.LaterHours * float64(time.Hour))
		}
		if due > laterMax {
			due = laterMax
		}
		err := m.Memory.AddThread(s.GuildID, memory.Thread{
			Due: now.Add(due), Person: memory.Ref{ID: s.UserID, Name: s.Username}, Text: a.Later,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func hasNote(notes []memory.Note, text string) bool {
	for _, n := range notes {
		if strings.EqualFold(oneLine(n.Text), oneLine(text)) {
			return true
		}
	}
	return false
}

// refs is who a moment is with: the person it is about.
func (m *Mind) refs(s Scene) []memory.Ref {
	if s.UserID == "" {
		return nil
	}
	return []memory.Ref{{ID: s.UserID, Name: s.Username}}
}

// Said records something she said, with what she meant by it and what it
// answered.
//
// Every message she sends goes through here. It is the single most important
// write in the package: it is what lets her own a thing she said yesterday
// instead of reading it back as someone else's. See docs/persona.md.
func (m *Mind) Said(s Scene, a Appraisal, text, why string) error {
	var b strings.Builder
	quoted := clip(oneLine(text), maxMomentChars)
	switch {
	case Initiated(s.Trigger):
		if s.Username != "" && s.Trigger == TriggerReach {
			fmt.Fprintf(&b, "I went to %s myself and said: %q", s.Username, quoted)
		} else {
			fmt.Fprintf(&b, "I spoke up on my own: %q", quoted)
		}
		if why != "" {
			b.WriteString(" — because " + oneLine(why))
		}
	default:
		if said := theirLine(s); said != "" {
			fmt.Fprintf(&b, "%s: %q → I said: %q", nameOr(s.Username), clip(said, 160), quoted)
		} else {
			fmt.Fprintf(&b, "said to %s: %q", nameOr(s.Username), quoted)
		}
	}
	if a.Intent != "" && !Initiated(s.Trigger) {
		b.WriteString(" — meant: " + oneLine(a.Intent))
	}
	return m.Memory.AddMoment(s.GuildID, memory.Moment{
		At: s.Now, Channel: s.ChannelName, People: m.refs(s), Text: b.String(),
	})
}

// LetGo records a moment she chose not to answer in words, with why. Asked
// later why she ignored someone, she then has an answer that is true.
func (m *Mind) LetGo(s Scene, a Appraisal) error {
	said := theirLine(s)
	if said == "" || s.Trigger == TriggerOverheard {
		return nil
	}
	what := "I let it go"
	if a.Act == ActReact {
		what = "I only reacted " + a.Emoji
	}
	text := fmt.Sprintf("%s: %q → %s", nameOr(s.Username), clip(said, 160), what)
	if a.Read != "" {
		text += " — I read it as: " + oneLine(a.Read)
	}
	return m.Memory.AddMoment(s.GuildID, memory.Moment{
		At: s.Now, Channel: s.ChannelName, People: m.refs(s), Text: text,
	})
}

// theirLine is what the person she is answering said since she last spoke,
// run together: a burst is one approach.
func theirLine(s Scene) string {
	var parts []string
	for i := len(s.Turns) - 1; i >= 0; i-- {
		t := s.Turns[i]
		if t.FromBot {
			break
		}
		if t.UserID == s.UserID {
			parts = append([]string{oneLine(t.Content)}, parts...)
		}
	}
	return strings.Join(parts, " / ")
}

func nameOr(name string) string {
	if name == "" {
		return "someone"
	}
	return name
}
