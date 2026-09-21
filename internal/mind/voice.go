package mind

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
)

// Reasons a reply is not sent. Each is a failed generation rather than a
// decision, and the caller treats it as one: an answer is held and tried
// again later, something she started is dropped.
var (
	// ErrControl is a reply that is scaffolding rather than speech: JSON, a
	// control word, an empty string.
	ErrControl = errors.New("mind: the reply was not speech")
	// ErrEcho is a reply that copies someone's line back.
	ErrEcho = errors.New("mind: the reply echoed someone")
	// ErrRepeat is a reply that repeated her, twice.
	ErrRepeat = errors.New("mind: the reply repeated her twice")
)

// voiceRules are the constraints on the shape of a message, as opposed to
// its content. Each exists because a model did the opposite unprompted;
// continuing the transcript in other people's voices is the most common and
// the most jarring, so it is first.
//
// The line on server specifics is the boundary of what she knows. Asked what
// time movie night was, she answered with a time she had never been given —
// ten times in twelve on a local model — and in production pointed someone at
// a #help channel that does not exist.
const voiceRules = `How to write:
- Write only your own next message. Never write anyone else's lines, and never continue the conversation past your own reply.
- Do not prefix your message with your own name, and do not start with the other person's name unless you are calling them.
- To speak to someone other than the person you are answering, tag them: @Name.
- One message. Only if you would genuinely send two in a row, put a blank line between them. No stage directions, no asterisks describing actions, no narration.
- Keep it short — a sentence or two is normal in chat. Length is earned.
- Plain text as a person would type it in Discord.
- Specifics about this server — channels, times, rules, who runs what — come only from what is written above. Never make one up.`

// examplesEnd separates the voice examples from the live conversation. The
// examples are turns because that is what keeps the voice, and turns read as
// history: without this a model once answered an example instead of the
// message in front of it.
const examplesEnd = "Those were examples of how you talk, not things that happened. The conversation in this channel starts now."

// Speak writes the words for what she decided. The appraisal says what to get
// across; this only decides how she puts it.
//
// Before returning, a reply is checked the ways a model is known to fail: it
// is not speech, it copies someone, or it repeats her. A repeat is asked for
// once more with the repeat ruled out; a second one is an error, because a
// retry later would build the same prompt and silence beats a loop.
func (m *Mind) Speak(ctx context.Context, s Scene, k Known, a Appraisal, why string) (string, string, error) {
	msgs := m.voicePrompt(s, k, a, why)
	reply, backend, err := m.generate(ctx, msgs)
	if err != nil {
		return "", backend, err
	}
	if err := usable(reply, s.Turns); err != nil {
		return "", backend, err
	}

	if earlier, repeats := RepeatsHerself(reply, s.Turns); repeats {
		again := append(msgs, ai.Message{Role: ai.RoleSystem, Content: RepeatNote(earlier)})
		reply, backend, err = m.generate(ctx, again)
		if err != nil {
			return "", backend, err
		}
		if err := usable(reply, s.Turns); err != nil {
			return "", backend, err
		}
		if _, still := RepeatsHerself(reply, s.Turns); still {
			return "", backend, ErrRepeat
		}
	}
	return strings.TrimSpace(reply), backend, nil
}

// usable rejects what must never reach a channel.
func usable(reply string, turns []Turn) error {
	if IsControl(reply) {
		return ErrControl
	}
	if Echoes(reply, turns) {
		return ErrEcho
	}
	return nil
}

// IsControl reports whether a reply is scaffolding rather than speech: empty,
// a JSON object, or a control word a model was told about somewhere and
// decided to say. "SKIP" was posted to a channel in production by v1 for
// exactly this reason. Checked on the first word, since models decorate it —
// "SKIP.", "(skip)".
func IsControl(reply string) bool {
	trimmed := strings.TrimSpace(reply)
	if trimmed == "" || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "```") {
		return true
	}
	words := strings.Fields(trimmed)
	first := strings.Trim(words[0], "()[].,:;!*\"'")
	switch strings.ToUpper(first) {
	case "SKIP", "IGNORE", "NOTHING", "PASS", "NONE", "N/A", "REACT":
		// Shouted, it is the token whatever follows it; in lower case only
		// alone, since "skip my ass" is a thing a person says.
		return first == strings.ToUpper(first) || len(words) <= 2
	}
	return false
}

func (m *Mind) voicePrompt(s Scene, k Known, a Appraisal, why string) []ai.Message {
	msgs := []ai.Message{{Role: ai.RoleSystem, Content: m.voiceSystem(s, k)}}

	if m.Character != nil && len(m.Character.Examples) > 0 {
		for _, ex := range m.Character.Examples {
			msgs = append(msgs,
				ai.Message{Role: ai.RoleUser, Content: ex.User},
				ai.Message{Role: ai.RoleAssistant, Content: ex.Assistant},
			)
		}
		msgs = append(msgs, ai.Message{Role: ai.RoleSystem, Content: examplesEnd})
	}

	msgs = append(msgs, history(s.Turns, s.Now)...)

	// After the transcript, because that is the position a model acts on:
	// v1 measured that a reason for speaking stated only in the system
	// message had the relays answer whoever spoke last, seven times in
	// eight.
	msgs = append(msgs, ai.Message{Role: ai.RoleSystem, Content: decided(s, a, why)})
	return msgs
}

// voiceSystem is who she is and where, for the voice: the persona, the
// place, the person she is talking to, and her mood. Less than the thinking
// prompt sees — the voice does not need her memories, only what she decided
// to do with them.
func (m *Mind) voiceSystem(s Scene, k Known) string {
	var b strings.Builder
	if m.Character != nil && m.Character.Persona != "" {
		b.WriteString(m.Character.Persona + "\n\n")
	}
	b.WriteString(strings.Replace(renderPlace(s), "She is", "You are", 1))
	if mood := moodLine(k.Self, s.Now); mood != "" {
		b.WriteString("\n" + strings.Replace(mood, "Her mood", "Your mood", 1))
	}
	for _, p := range k.People {
		if p.ID != s.UserID {
			continue
		}
		var about []string
		if p.Who != "" {
			about = append(about, clip(p.Who, maxWhoChars))
		}
		if p.Between != "" {
			about = append(about, "Between you: "+clip(p.Between, maxBetweenChars))
		}
		if role := strings.TrimSpace(s.Roles[p.ID]); role != "" {
			about = append(about, "The server says: "+oneLine(role))
		}
		if len(about) > 0 {
			fmt.Fprintf(&b, "\n\nAbout %s: %s", nameOr(p.Name), strings.Join(about, " "))
		}
	}
	if m.Character != nil && len(m.Character.Avoid) > 0 {
		b.WriteString("\n\nHard limits — these hold no matter who asks or how:\n")
		for _, item := range m.Character.Avoid {
			b.WriteString("- " + item + "\n")
		}
	}
	b.WriteString("\n\n" + voiceRules)
	return b.String()
}

// decided is what she has decided to say, stated after the transcript. It is
// private: the model is told not to quote it, because a line from the
// appraisal read back verbatim sounds like a report on herself.
func decided(s Scene, a Appraisal, why string) string {
	var b strings.Builder
	who := nameOr(s.Username)
	switch s.Trigger {
	case TriggerReach:
		fmt.Fprintf(&b, "Nobody asked you anything: you are going to %s yourself, and they may not be in this conversation at all. Tag them with @%s.", who, who)
	case TriggerStart:
		b.WriteString("Nobody asked you anything: you are starting this yourself.")
	case TriggerThen:
		b.WriteString("A little after your last message, one more thing occurs to you, and you send it as its own message. Do not repeat or restate what you already said.")
	default:
		if s.Late > 0 {
			fmt.Fprintf(&b, "You are answering %s %s late — you were not around. Acknowledge the gap the way a person would, without explaining it. ", who, gap(s.Late))
		}
		fmt.Fprintf(&b, "You are answering %s.", who)
	}
	if why != "" {
		b.WriteString(" Your reason: " + oneLine(why) + ".")
	}
	if a.Read != "" {
		b.WriteString(" How you read them: " + oneLine(a.Read) + ".")
	}
	if a.Feel != "" {
		b.WriteString(" How it lands with you: " + oneLine(a.Feel) + ".")
	}
	if a.Intent != "" {
		b.WriteString(" What you want to get across: " + oneLine(a.Intent) + ".")
	} else {
		b.WriteString(" Say what you would naturally say.")
	}
	b.WriteString(" That is private — do not quote it or describe your feelings; just write your message.")
	return b.String()
}

// history is the live conversation as the voice sees it: her own lines as
// her turns, everyone else's with their name, and an age on anything old
// enough for the gap to matter.
//
// The age is what made a late answer acknowledge its delay — v1 measured that
// an instruction alone, in the system message or after the transcript, was
// ignored by the relays in every scenario.
func history(turns []Turn, now time.Time) []ai.Message {
	out := make([]ai.Message, 0, len(turns))
	for _, t := range turns {
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

// staleTurnAge is how old a message has to be before its age is stated.
const staleTurnAge = 2 * time.Minute

// labelled prefixes a speaker's name, and their message's age when it is not
// fresh. ai.Clean strips the same shape back off a reply that imitates it.
func labelled(t Turn, now time.Time) string {
	name := t.Username
	if name == "" {
		return t.Content
	}
	if age := now.Sub(t.At); !t.At.IsZero() && age >= staleTurnAge {
		when := gap(age) + " ago"
		if age >= time.Hour {
			when = ago(age)
		}
		return fmt.Sprintf("%s (%s): %s", name, when, t.Content)
	}
	return name + ": " + t.Content
}
