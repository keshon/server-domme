package mind

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/keshon/server-domme/internal/ai"
)

// Style is the shape of her messages that the code checks after she has
// written one: how long she ever goes, measured from her authored examples,
// and the phrases the author says are not hers. Both are facts about text,
// counted, never a reading of it — the same kind of check as a repeat, and
// never stated to the model as a number or a rule ahead of time.
//
// A reply off the shape is asked for once more, with the reason named; the
// closer of the two is kept, and what is still too long is cut at a sentence
// boundary, the way someone deletes the second half before sending.
//
// A third question in a row is off the shape too. Asking back after every
// answer is an interview, and the most assistant-like thing a character can
// do: in production, told about a music bot, she asked seven questions in
// seven replies and gave no take of her own. Counted over her own lines in
// the conversation, not over what they mean.
//
// Casual does the surface — punctuation, contractions; this does the length,
// the words and the questions. See docs/persona-v3.md, G.
type Style struct {
	// MaxWords is the longest she writes: her longest example, stretched.
	// Zero checks no length.
	MaxWords int
	// NotHers are phrases she never uses, lowercased.
	NotHers []string
}

// How far past her longest example a reply may run. Examples are a sample,
// not a ceiling: a reply half as long again as anything in them is still
// hers, and one with nothing to cut would not be cut anyway.
const (
	styleStretch  = 1.5
	minStyleWords = 30
)

// Style measures the style from the character file.
func (c *Character) Style() Style {
	var st Style
	if c == nil {
		return st
	}
	longest := 0
	for _, ex := range c.Examples {
		longest = max(longest, len(strings.Fields(ex.Assistant)))
	}
	if longest > 0 {
		st.MaxWords = max(minStyleWords, int(float64(longest)*styleStretch))
	}
	for _, p := range c.NotHers {
		if p = normalizePhrase(p); p != "" {
			st.NotHers = append(st.NotHers, p)
		}
	}
	return st
}

// offStyle is how a reply misses the style.
type offStyle struct {
	// Phrase is the first phrase in it that is not hers.
	Phrase string
	// Long is that it runs past MaxWords with no sentence boundary to cut
	// at inside it — the part a cut cannot fix.
	Long bool
	// Questions is that it ends in a question after questionRun of her
	// lines in a row already did.
	Questions bool
	// Words is its length.
	Words int
}

func (o offStyle) misses() int {
	n := 0
	if o.Phrase != "" {
		n++
	}
	if o.Long {
		n++
	}
	if o.Questions {
		n++
	}
	return n
}

// questionRun is how many of her lines in a row may end in a question before
// one more is off her style.
const questionRun = 2

// askedInARow reports whether her last questionRun lines in the conversation
// all ended in a question.
func askedInARow(turns []Turn) bool {
	n := 0
	for i := len(turns) - 1; i >= 0 && n < questionRun; i-- {
		if !turns[i].FromBot {
			continue
		}
		if !endsInQuestion(turns[i].Content) {
			return false
		}
		n++
	}
	return n == questionRun
}

// endsInQuestion reports whether text ends with a question mark, past any
// closing quote or markup.
func endsInQuestion(text string) bool {
	return strings.HasSuffix(strings.TrimRight(text, " \t\n)\"'*_"), "?")
}

// dropQuestions cuts the questions a reply ends with, when something comes
// before them: "that's a sound approach. how do you balance it?" is sent as
// "that's a sound approach." A reply that is nothing but a question is left
// as it is.
func dropQuestions(reply string) string {
	reply = strings.TrimSpace(reply)
	if !endsInQuestion(reply) {
		return reply
	}
	ends := append(sentenceEnds(reply), len(reply))
	for i := len(ends) - 2; i >= 0; i-- {
		head := strings.TrimSpace(reply[:ends[i]])
		if head == "" {
			break
		}
		if !endsInQuestion(head) {
			return head
		}
	}
	return reply
}

// check measures a reply against the style.
func (st Style) check(reply string) offStyle {
	o := offStyle{Words: len(strings.Fields(reply))}
	text := normalizePhrase(reply)
	for _, p := range st.NotHers {
		if containsPhrase(text, p) {
			o.Phrase = p
			break
		}
	}
	if st.MaxWords > 0 && o.Words > st.MaxWords && st.cut(reply) == strings.TrimSpace(reply) {
		o.Long = true
	}
	return o
}

// cut shortens a reply that runs past MaxWords to the longest run of whole
// sentences within it. One whose first sentence is already too long is left
// as it is: cutting a sentence in half is worse than a long message.
func (st Style) cut(reply string) string {
	reply = strings.TrimSpace(reply)
	if st.MaxWords <= 0 || len(strings.Fields(reply)) <= st.MaxWords {
		return reply
	}
	best := ""
	for _, end := range sentenceEnds(reply) {
		head := strings.TrimSpace(reply[:end])
		if len(strings.Fields(head)) > st.MaxWords {
			break
		}
		best = head
	}
	if best == "" {
		return reply
	}
	return best
}

// sentenceEnds are the offsets just past each sentence in text: after a
// full stop, question or exclamation mark followed by a space, and at each
// line break.
func sentenceEnds(text string) []int {
	var ends []int
	runes := []rune(text)
	offset := 0
	for i, r := range runes {
		offset += len(string(r))
		switch {
		case r == '\n':
			ends = append(ends, offset)
		case r == '.' || r == '?' || r == '!':
			if i+1 < len(runes) && unicode.IsSpace(runes[i+1]) {
				ends = append(ends, offset)
			}
		}
	}
	return ends
}

// normalizePhrase lowercases and straightens quotes, so a phrase in the
// card matches however the model typed it.
func normalizePhrase(s string) string {
	return strings.TrimSpace(strings.ToLower(typographic.Replace(s)))
}

// containsPhrase reports whether text has phrase as whole words: "honestly"
// is not in "dishonestly".
func containsPhrase(text, phrase string) bool {
	for from := 0; ; {
		i := strings.Index(text[from:], phrase)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(phrase)
		if !wordRuneBefore(text, i) && !wordRuneAfter(text, end) {
			return true
		}
		from = i + 1
	}
}

func wordRuneBefore(text string, i int) bool {
	if i == 0 {
		return false
	}
	r := []rune(text[:i])
	return isWordRune(r[len(r)-1])
}

func wordRuneAfter(text string, i int) bool {
	if i >= len(text) {
		return false
	}
	return isWordRune([]rune(text[i:])[0])
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// styleNote is what the voice is told when asked again, after the reply
// that missed: what was wrong with that message, as a fact about it.
func styleNote(o offStyle) string {
	var parts []string
	if o.Phrase != "" {
		parts = append(parts, fmt.Sprintf("it says %q, which is not something you say", o.Phrase))
	}
	if o.Long {
		parts = append(parts, "it is far longer than anything you write in chat")
	}
	if o.Questions {
		parts = append(parts, "it ends in a question, and so did each of your last messages here")
	}
	return "You were about to send that, and did not: " + strings.Join(parts, ", and ") +
		". Write your message again, the way you would actually type it."
}

// restyle checks a reply against her style. One that misses is asked for
// once more; the retry is kept if it misses less, or as much but shorter,
// and passes the checks every reply passes. Whatever is kept is cut to
// length. Each miss is logged: how often a backend misses is a measurement.
func (m *Mind) restyle(ctx context.Context, s Scene, msgs []ai.Message, reply, backend string) (string, string) {
	st := m.Character.Style()
	asked := askedInARow(s.Turns)
	check := func(r string) offStyle {
		o := st.check(r)
		o.Questions = asked && endsInQuestion(r)
		return o
	}
	first := check(reply)
	if first.misses() == 0 {
		return st.cut(reply), backend
	}
	kept := false
	again := append(msgs, ai.Message{Role: ai.RoleAssistant, Content: reply}, ai.Message{Role: ai.RoleSystem, Content: styleNote(first)})
	if retry, b, err := m.speak(ctx, again); err == nil && usable(retry, s.Turns) == nil {
		if _, repeats := RepeatsHerself(retry, s.Turns); !repeats {
			second := check(retry)
			if second.misses() < first.misses() || (second.misses() == first.misses() && second.Words < first.Words) {
				reply, backend, kept = retry, b, true
			}
		}
	}
	m.Log.Info().
		Str("guild_id", s.GuildID).
		Str("backend", backend).
		Str("phrase", first.Phrase).
		Bool("long", first.Long).
		Bool("questions", first.Questions).
		Int("words", first.Words).
		Bool("retry_kept", kept).
		Msg("mind_reply_restyled")
	if check(reply).Questions {
		reply = dropQuestions(reply)
	}
	return st.cut(reply), backend
}
