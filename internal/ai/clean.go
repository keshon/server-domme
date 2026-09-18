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
	// Checked first. The speaker-prefix strip below removes a one-word label
	// and its colon, so by the time it has run "Moderation: ok" is just "ok"
	// and indistinguishable from speech.
	if isControlArtifact(reply) {
		return ""
	}

	reply = thinkBlock.ReplaceAllString(reply, "")
	reply = unclosedThink.ReplaceAllString(reply, "")
	reply = strings.TrimSpace(reply)

	reply = speakerPrefix.ReplaceAllString(reply, "")
	reply = strings.TrimSpace(reply)

	reply = cutContinuedConversation(reply)
	reply = stripWrappingQuotes(reply)

	return truncate(reply, MaxReplyChars)
}

// controlArtifact matches a relay leaking its own moderation scaffolding as a
// reply — "User Safety: safe" — which arrives as a perfectly well-formed HTTP
// 200 and reaches the channel verbatim. Observed repeatedly from one backend
// while testing the character.
//
// Matched on the known vocabulary rather than on the shape. The first attempt
// matched any short "Label: value" and threw away "the answer is: no", which
// is a real thing to say; a false positive here is silence with no explanation,
// so the guard is narrow and lets an unknown artifact through rather than
// risking that.
//
// Applied per line, and a reply is dropped only when every line is one. A
// safety-guard model behind one relay answered with its whole verdict —
// "User Safety: unsafe / Response Safety: unsafe / Safety Categories:
// Profanity, Harassment" — which the single-line version let through.
var controlArtifact = regexp.MustCompile(
	`(?i)^\s*((user|response|prompt)\s+)?(safety(\s+categor(y|ies))?|moderation|content\s+filter|policy|compliance|flagged)\s*:\s*[\w-]+(\s*,\s*[\w-]+( [\w-]+)?)*\s*$`)

// toolCallMarkup is a relay leaking the scaffolding of a tool-calling model:
// "<tool_call>The question is: ..." arrived as a whole reply while probing the
// character. Nothing she says contains these tags, so their presence anywhere
// means the rest is scaffolding too.
var toolCallMarkup = regexp.MustCompile(`(?i)</?(tool_call|function_call|tool_use)\b`)

func isControlArtifact(reply string) bool {
	if toolCallMarkup.MatchString(reply) {
		return true
	}
	lines := 0
	for _, line := range strings.Split(reply, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !controlArtifact.MatchString(line) {
			return false
		}
		lines++
	}
	return lines > 0
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
