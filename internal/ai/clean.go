package ai

import (
	"regexp"
	"strings"
	"unicode"
)

// MaxReplyChars is Discord's per-message content limit. A longer reply is
// refused by the API outright, so it is cut here rather than at send time —
// the old chat code truncated at 2800 and every long reply it produced failed
// to post.
const MaxReplyChars = 2000

// truncationMark replaces the tail of an over-long reply. It is deliberately
// visible: a reply that simply stops mid-sentence reads as the bot losing
// interest, which is a character trait we do not want to fake by accident.
const truncationMark = "…"

// thinkBlock matches the reasoning blocks that open-weight models emit around
// their scratch work. Several of the relayed models are reasoning models, and
// they leak these into content rather than into a separate field.
var thinkBlock = regexp.MustCompile(`(?s)<(think|thinking|reasoning)>.*?</(think|thinking|reasoning)>`)

// unclosedThink matches a reasoning block whose closing tag never arrived,
// which happens when a reply is cut off at the token limit mid-thought.
// Everything from the opening tag on is scratch work, so it all goes.
var unclosedThink = regexp.MustCompile(`(?s)<(think|thinking|reasoning)>.*$`)

// speakerPrefix matches a leading "Name:" attribution.
//
// The conversation is handed to the model as "username: text" lines — with
// "username (5 minutes ago): text" for older ones — so it can tell
// participants apart and tell what is recent. Models reliably imitate that
// shape in their own output. Left alone it reaches the channel as "Domme: hey"
// under a username that already says Domme, so both forms are stripped.
var speakerPrefix = regexp.MustCompile(`^[^\s:]{1,32}(\s*\([^)]{1,40}\))?:\s+`)

// continuedSpeaker matches a line that starts a new speaker's turn.
//
// The prompt forbids it twice and the models do it anyway: a reply ends, then
// a blank line, then "someone-else: ..." carrying on the conversation in a
// real member's voice. Posted verbatim that puts words in a named person's
// mouth, which is the worst thing this bot could do to a channel, so the
// reply is cut at the first such line.
//
// The trade is that a legitimate line opening with a bare word and a colon —
// "edit: ..." — is also cut. That is worth it: losing a trailing clause costs
// a little, and impersonating a member costs a lot.
var continuedSpeaker = regexp.MustCompile(`(?m)^[^\s:]{1,32}(\s*\([^)]{1,40}\))?:\s`)

// quotePairs are the wrappers a model puts around an utterance when it has
// been asked to speak as someone. Only a matched pair is stripped.
var quotePairs = [][2]string{
	{`"`, `"`},
	{`'`, `'`},
	{"«", "»"},
	{"“", "”"},
	{"‘", "’"},
}

// Clean turns raw model output into something postable to a channel.
//
// Every transformation here removes an artefact of how the model was prompted
// rather than editing what it said: reasoning traces, the speaker label copied
// from the conversation format, and quotation marks around the whole
// utterance. Do not add tone or content filtering here — that belongs in the
// prompt, where the model can act on it, not in a regex that silently mangles
// a reply the character meant to give.
func Clean(reply string) string {
	reply = thinkBlock.ReplaceAllString(reply, "")
	reply = unclosedThink.ReplaceAllString(reply, "")
	reply = strings.TrimSpace(reply)

	reply = speakerPrefix.ReplaceAllString(reply, "")
	reply = strings.TrimSpace(reply)

	reply = cutContinuedConversation(reply)
	reply = stripWrappingQuotes(reply)

	return truncate(reply, MaxReplyChars)
}

// stripWrappingQuotes removes one matched pair enclosing the whole reply. A
// reply that merely contains a quote keeps it: the pair has to be at both ends
// and nowhere else in between, or stripping it would join two quoted lines
// into one unquoted run.
func stripWrappingQuotes(reply string) string {
	for _, pair := range quotePairs {
		open, close := pair[0], pair[1]
		if len(reply) <= len(open)+len(close) {
			continue
		}
		if !strings.HasPrefix(reply, open) || !strings.HasSuffix(reply, close) {
			continue
		}
		inner := reply[len(open) : len(reply)-len(close)]
		if strings.Contains(inner, open) || strings.Contains(inner, close) {
			continue
		}
		return strings.TrimSpace(inner)
	}
	return reply
}

// truncate cuts a reply to max runes, preferring the last sentence or word
// boundary so the cut does not land inside a word.
func truncate(reply string, max int) string {
	runes := []rune(reply)
	if len(runes) <= max {
		return reply
	}

	cut := string(runes[:max-len([]rune(truncationMark))])

	if idx := strings.LastIndexAny(cut, ".!?"); idx > len(cut)/2 {
		return strings.TrimSpace(cut[:idx+1])
	}
	if idx := strings.LastIndexFunc(cut, unicode.IsSpace); idx > len(cut)/2 {
		return strings.TrimSpace(cut[:idx]) + truncationMark
	}
	return strings.TrimSpace(cut) + truncationMark
}

// cutContinuedConversation drops everything from the point the reply starts
// speaking as somebody else. See continuedSpeaker.
func cutContinuedConversation(reply string) string {
	loc := continuedSpeaker.FindStringIndex(reply)
	if loc == nil {
		return reply
	}
	// A match at the very start is the reply labelling itself, which
	// speakerPrefix has already dealt with; anything left there is the whole
	// reply and cutting it would leave nothing.
	if loc[0] == 0 {
		return reply
	}
	return strings.TrimSpace(reply[:loc[0]])
}
