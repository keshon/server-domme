package mind

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
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
// retry later would build the same prompt and silence beats a loop. Last, with
// StyleCheck on, it is held to her style; see restyle.
func (m *Mind) Speak(ctx context.Context, s Scene, k Known, a Appraisal, why string) (string, string, error) {
	examples := m.sampleExamples(s.ShortExamples, situationOf(s, a))
	k = m.fit(s.GuildID, "voice", k, m.voiceBudget(), func(k Known) int { return promptSize(m.voicePrompt(s, k, a, why, examples)) })
	msgs := m.voicePrompt(s, k, a, why, examples)
	seen := heard(s, k)
	reply, backend, err := m.speak(ctx, msgs)
	if err != nil {
		return "", backend, err
	}
	if err := usable(reply, seen); err != nil {
		return "", backend, err
	}

	if earlier, repeats := RepeatsHerself(reply, s.Turns); repeats {
		again := append(msgs, ai.Message{Role: ai.RoleSystem, Content: RepeatNote(earlier)})
		reply, backend, err = m.speak(ctx, again)
		if err != nil {
			return "", backend, err
		}
		if err := usable(reply, seen); err != nil {
			return "", backend, err
		}
		if _, still := RepeatsHerself(reply, s.Turns); still {
			return "", backend, ErrRepeat
		}
	}
	if m.StyleCheck {
		reply, backend = m.restyle(ctx, s, msgs, reply, backend, seen)
	}
	return strings.TrimSpace(reply), backend, nil
}

// heard is what other people said that the voice was shown: the live
// conversation, and the lines quoted in the memories it was given. A line
// from a memory is as easy to copy as one from the transcript: in production
// she answered Big M with his own words from ninety minutes before — "you
// are very kind... Server Domme cough-cough" — which was in her recall and
// long out of the conversation.
func heard(s Scene, k Known) []Turn {
	out := append([]Turn(nil), s.Turns...)
	for _, mo := range topRecalled(k.Recalled, voiceRecalled) {
		text, ok := observedPart(mo)
		if !ok {
			continue
		}
		open, end := strings.Index(text, `"`), strings.LastIndex(text, `"`)
		if open < 0 || end <= open {
			continue
		}
		quoted := strings.ReplaceAll(text[open+1:end], `\"`, `"`)
		for _, line := range strings.Split(quoted, theirJoin) {
			if line = strings.TrimSpace(line); line != "" && line != "…" {
				out = append(out, Turn{Content: line})
			}
		}
	}
	return out
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

func (m *Mind) voicePrompt(s Scene, k Known, a Appraisal, why string, examples []Exchange) []ai.Message {
	msgs := []ai.Message{{Role: ai.RoleSystem, Content: m.voiceSystem(s, k)}}

	if len(examples) > 0 {
		for _, ex := range examples {
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

// voiceSystem is who she is and where, for the voice: the persona and her
// specifics, the place, her mood, the person she is talking to with the last
// few things she noted about them, and the few memories and things she has
// said about herself that bear on this.
//
// v2 gave the voice only the person's paragraphs and the decided gist, so any
// detail — how the naming of his project went, the thing she said she hates
// — reached it only if the gist happened to carry it. Details are what make a
// line read as someone's. See docs/persona-v3.md, G.
func (m *Mind) voiceSystem(s Scene, k Known) string {
	var b strings.Builder
	if m.Character != nil && m.Character.Persona != "" {
		b.WriteString(m.Character.Persona + "\n\n")
	}
	if sp := renderSpecifics("Specifically, about you:", k.Specifics); sp != "" {
		b.WriteString(sp + "\n\n")
	}
	b.WriteString(strings.Replace(renderPlace(s), "She is", "You are", 1))
	if body := renderBody(s, "You", "have"); body != "" {
		b.WriteString(" " + body)
	}
	if r := renderReactions(s.Reactions, "your"); r != "" {
		b.WriteString("\n" + r)
	}
	if mood := moodLine(k.Self, s.Now); mood != "" {
		b.WriteString("\n" + strings.Replace(mood, "Her mood", "Your mood", 1))
	}
	if feelings := renderFeelings(k.Self.Feelings, s.Now, "you"); feelings != "" {
		b.WriteString("\n" + feelings)
	}
	if onMind := renderOnMind(k.Self, s.Now, "your"); onMind != "" {
		b.WriteString("\n" + onMind)
	}
	if life := renderLife(freshLife(k.Self.Life, s.Now)); life != "" {
		b.WriteString("\n" + strings.Replace(life, "her days", "your days", 1))
	}
	for _, p := range k.People {
		if p.ID != s.UserID {
			continue
		}
		var about []string
		if p.Pronouns != "" {
			about = append(about, "("+oneLine(p.Pronouns)+")")
		}
		if p.Who != "" {
			about = append(about, clip(p.Who, maxWhoChars))
		}
		if p.Between != "" {
			about = append(about, "Between you: "+clip(p.Between, maxBetweenChars))
		}
		if p.Works != "" {
			about = append(about, "What works with them: "+clip(p.Works, maxBetweenChars))
		}
		if role := strings.TrimSpace(s.Roles[p.ID]); role != "" {
			about = append(about, "The server says: "+oneLine(role))
		}
		if len(about) > 0 {
			fmt.Fprintf(&b, "\n\nAbout %s: %s", nameOr(p.Name), strings.Join(about, " "))
		}
		if notes := lastNotes(p.Notes, voiceNotes); len(notes) > 0 {
			fmt.Fprintf(&b, "\nWhat you have noted about %s:", nameOr(p.Name))
			for _, n := range notes {
				b.WriteString("\n- " + dated(n, s.Now))
			}
		}
	}
	if recalled := topRecalled(k.Recalled, voiceRecalled); len(recalled) > 0 {
		b.WriteString("\n\nWhat you remember that may bear on this:")
		for _, mo := range recalled {
			b.WriteString("\n- " + renderMoment(mo, s.Now))
		}
	}
	if facts := renderSelfFacts("Things you have said about yourself that bear on this:", k.SelfFacts); facts != "" {
		b.WriteString("\n\n" + facts)
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

// How much of her memory the voice sees: a few notes on the person and the
// few memories that came back strongest. The thinking call sees more; the
// voice needs the details, not the file.
const (
	voiceNotes    = 3
	voiceRecalled = 3
)

// lastNotes is the most recent n notes.
func lastNotes(notes []memory.Note, n int) []memory.Note {
	if len(notes) > n {
		return notes[len(notes)-n:]
	}
	return notes
}

// topRecalled is the n moments recall brought back most strongly, oldest
// first.
func topRecalled(moments []memory.Moment, n int) []memory.Moment {
	if len(moments) <= n {
		return moments
	}
	byScore := append([]memory.Moment(nil), moments...)
	sort.SliceStable(byScore, func(i, j int) bool { return byScore[i].Score > byScore[j].Score })
	top := byScore[:n]
	sort.SliceStable(top, func(i, j int) bool { return top[i].At.Before(top[j].At) })
	return top
}

// sampleExamples is the authored examples the voice is shown this time: a
// random ExamplesSample of them, in the author's order, or all of them.
//
// Every example replayed on every call teaches whatever they have in common
// as a template; drawn afresh each time, what they share is the voice and
// what varies stays varied. See docs/persona-v3.md, G.
//
// With a situation, most of the sample is examples filed under it, placed
// last, nearest the conversation; the rest are drawn from the others, so one
// kind of moment does not collapse into one line. With none filed under it,
// the sample is drawn from all of them.
//
// short draws from the shorter half of her replies only: when the body has
// little energy left, the voice she hears in them is the terse one.
func (m *Mind) sampleExamples(short bool, sit Situation) []Exchange {
	if m.Character == nil {
		return nil
	}
	all := m.Character.Examples
	if short {
		all = shorterHalf(all)
	}
	n := m.ExamplesSample
	if n <= 0 || n >= len(all) {
		return all
	}
	var matched, rest []int
	for i, ex := range all {
		if sit != "" && ex.fits(sit) {
			matched = append(matched, i)
		} else {
			rest = append(rest, i)
		}
	}
	if len(matched) == 0 {
		return pickExchanges(all, m.pick(rest, n))
	}
	take := min(len(matched), situationShare(n))
	others := m.pick(rest, min(n-take, len(rest)))
	fitting := m.pick(matched, n-len(others))
	return append(pickExchanges(all, others), pickExchanges(all, fitting)...)
}

// situationShare is how many of a sample of n go to the situation in front
// of her: three in four, rounded up.
func situationShare(n int) int { return (3*n + 3) / 4 }

// fits reports whether an example is filed under a situation.
func (ex Exchange) fits(sit Situation) bool {
	for _, s := range ex.Situations {
		if s == sit {
			return true
		}
	}
	return false
}

// pick draws k of the indices at random, and returns them in order.
func (m *Mind) pick(idx []int, k int) []int {
	idx = append([]int(nil), idx...)
	k = min(k, len(idx))
	for i := 0; i < k; i++ {
		j := i + int(m.roll()*float64(len(idx)-i))
		idx[i], idx[j] = idx[j], idx[i]
	}
	chosen := idx[:k]
	sort.Ints(chosen)
	return chosen
}

func pickExchanges(all []Exchange, idx []int) []Exchange {
	out := make([]Exchange, 0, len(idx))
	for _, i := range idx {
		out = append(out, all[i])
	}
	return out
}

// shorterHalf is the examples whose replies are no longer than the median.
func shorterHalf(all []Exchange) []Exchange {
	if len(all) <= 1 {
		return all
	}
	lengths := make([]int, len(all))
	for i, ex := range all {
		lengths[i] = len(ex.Assistant)
	}
	sorted := append([]int(nil), lengths...)
	sort.Ints(sorted)
	median := sorted[len(sorted)/2]
	var out []Exchange
	for i, ex := range all {
		if lengths[i] <= median {
			out = append(out, ex)
		}
	}
	return out
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
		if s.Username != "" {
			fmt.Fprintf(&b, "Nobody asked you anything: you are starting this yourself, with %s, who is around.", who)
		} else {
			b.WriteString("Nobody asked you anything: you are starting this yourself.")
		}
	case TriggerThen:
		b.WriteString("A little after your last message, one more thing occurs to you, and you send it as its own message. Do not repeat or restate what you already said.")
	case TriggerSight:
		fmt.Fprintf(&b, "Nobody asked you anything: %s is here, and you are bringing up something you meant to follow up on with them.", who)
	case TriggerLeave:
		b.WriteString("Nobody asked you anything: you are about to go. If you would say so, say it the way you would, in a few words.")
	case TriggerOverheard:
		b.WriteString("Nobody asked you anything: you are joining a conversation you overheard.")
	default:
		if s.Late > 0 {
			fmt.Fprintf(&b, "You are answering %s %s late — you were not around. Acknowledge the gap the way a person would, without explaining it. ", who, gap(s.Late))
		}
		fmt.Fprintf(&b, "You are answering %s.", who)
	}
	if why != "" {
		b.WriteString(" Your reason: " + oneLine(why) + ".")
	}
	// Only the gist, loosely. v2 also passed how she read them and how it
	// landed, which made the voice a renderer of an emotionally correct
	// answer; the reading is the thinking's, and the words are hers.
	if a.Intent != "" {
		b.WriteString(" Roughly what you want to get across: " + oneLine(a.Intent) + ".")
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
