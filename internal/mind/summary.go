package mind

import (
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
)

// Summary prompt shape.
const (
	gistPrefix   = "gist:"
	detailPrefix = "detail:"
	tonePrefix   = "tone:"
	// maxGistChars and maxDetailChars bound what is stored. A model told to
	// write one clause sometimes writes a paragraph, and this is the only
	// place that text is ever read back.
	maxGistChars   = 120
	maxDetailChars = 400
	// summaryTranscriptChars caps what is sent to be summarised. Larger than
	// the reply budget because a summary is worth more context than a single
	// answer, and it is asked for far less often.
	summaryTranscriptChars = 4000
)

// summaryInstruction asks for two labelled lines rather than JSON.
//
// The experiment this memory design came from asked for JSON and failed to
// parse 37% of the replies, against a local model it controlled. These
// backends are weaker and flakier than that. Two prefixed lines split on a
// colon, survive surrounding chatter, and degrade to "no memory this time"
// rather than to a parse error.
const summaryInstruction = `You are keeping a private note of what just happened in this channel, for your own memory later. Write exactly three lines and nothing else:

GIST: a single short clause naming what happened, under a dozen words
DETAIL: one or two sentences with the specifics worth remembering
TONE: one word — warm, ordinary, tense or hostile — for how it felt to be in

Write it as a note to yourself, not as a report to anyone. No preamble, no commentary, no quotation marks.`

// SummaryPrompt builds the request that turns a conversation into a memory.
//
// Deliberately not given the character file. This is not her speaking, it is a
// note being taken, and a persona in the prompt makes the model write the note
// in character — which produces a memory that is entertaining and vague rather
// than one that is useful weeks later.
func SummaryPrompt(turns []Turn) []ai.Message {
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
		transcript.WriteString(who)
		transcript.WriteString(": ")
		transcript.WriteString(t.Content)
		transcript.WriteString("\n")
	}

	return []ai.Message{
		{Role: ai.RoleSystem, Content: summaryInstruction},
		{Role: ai.RoleUser, Content: trimTo(transcript.String(), summaryTranscriptChars)},
	}
}

// ParseSummary pulls the gist and detail out of a reply.
//
// Tolerant on purpose: the labels may arrive in any case, wrapped in markdown,
// or with the model's own preamble above them. Only a missing gist is a
// failure, and the failure is silence rather than an error — a memory that
// cannot be read is simply not remembered.
func ParseSummary(reply string) (gist, detail string, tone Tone, ok bool) {
	tone = ToneOrdinary

	for _, line := range strings.Split(reply, "\n") {
		clean := strings.TrimSpace(line)
		clean = strings.Trim(clean, "*_`#-> ")
		lower := strings.ToLower(clean)

		switch {
		case strings.HasPrefix(lower, gistPrefix) && gist == "":
			gist = tidySummary(clean[len(gistPrefix):])
		case strings.HasPrefix(lower, detailPrefix) && detail == "":
			detail = tidySummary(clean[len(detailPrefix):])
		case strings.HasPrefix(lower, tonePrefix):
			tone = ParseTone(tidySummary(clean[len(tonePrefix):]))
		}
	}

	// A missing tone is not a failure. Asking for a third line makes a
	// malformed reply marginally likelier, and losing the whole memory over
	// the one optional field would be the wrong trade.
	if gist == "" {
		return "", "", ToneOrdinary, false
	}
	return trimTo(gist, maxGistChars), trimTo(detail, maxDetailChars), tone, true
}

// Tone is how a conversation felt, as the summariser read it.
//
// A closed vocabulary rather than free text, because this is acted on rather
// than displayed: four words map onto numbers, and anything a model invents
// outside them maps onto ToneOrdinary and changes nothing. It is the only
// judgement in this design a model is asked to make, and it is asked once per
// conversation rather than once per message — which is the difference between
// affordable and not.
type Tone string

const (
	ToneOrdinary Tone = "ordinary"
	ToneWarm     Tone = "warm"
	ToneTense    Tone = "tense"
	ToneHostile  Tone = "hostile"
)

// ParseTone maps a reported tone onto the vocabulary, defaulting to ordinary.
//
// Unrecognised input is not an error. A tone nobody can read means the
// conversation is remembered as unremarkable, which is the safe direction: the
// alternative is a misread word moving a dial nobody can trace.
func ParseTone(s string) Tone {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(ToneWarm):
		return ToneWarm
	case string(ToneTense):
		return ToneTense
	case string(ToneHostile):
		return ToneHostile
	default:
		return ToneOrdinary
	}
}

// WeighTone adjusts a computed weight by how the conversation felt.
//
// Length and headcount miss the short brutal exchange entirely, which is
// exactly the kind a person remembers longest. A charged conversation is more
// memorable whichever direction it was charged in, so warm raises it too.
func WeighTone(weight float64, tone Tone) float64 {
	switch tone {
	case ToneHostile:
		return clamp01(weight + 0.35)
	case ToneTense:
		return clamp01(weight + 0.15)
	case ToneWarm:
		return clamp01(weight + 0.10)
	default:
		return weight
	}
}

// tidySummary strips the punctuation a model wraps around a labelled value.
//
// Applied to the value and not only the line, because "**GIST:** the row"
// leaves its closing asterisks on the far side of the colon.
func tidySummary(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*_`")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'“”‘’`)
	return strings.TrimSpace(s)
}

// Weight scoring.
const (
	// busyEnoughToMatter is the turn count at which a conversation counts as
	// a real one rather than a passing exchange.
	busyEnoughToMatter = 20
	// crowdedEnough is the number of distinct voices that counts as everyone
	// being involved.
	crowdedEnough = 4
)

// WeighMoment scores how much a conversation mattered, 0..1.
//
// Computed here rather than asked of the model, for two reasons. Asking costs
// another line to parse, and the answer from a small model is closer to noise
// than to judgement — "rate the emotional weight of this conversation" is
// exactly the kind of question these backends answer confidently and
// arbitrarily. Length, how many people were drawn in, and how much of it was
// aimed at her are observable, stable, and about as predictive.
//
// It is not a claim about feelings. It decides how long a memory outlasts the
// ones either side of it; see Memory.Weight.
func WeighMoment(turns []Turn) float64 {
	if len(turns) == 0 {
		return 0
	}

	voices := make(map[string]bool, len(turns))
	var addressed, fromHer int
	for _, t := range turns {
		switch {
		case t.FromBot:
			fromHer++
		case t.UserID != "":
			voices[t.UserID] = true
		}
		if t.Mentioned {
			addressed++
		}
	}

	length := clamp01(float64(len(turns)) / busyEnoughToMatter)
	crowd := clamp01(float64(len(voices)) / crowdedEnough)
	involvement := clamp01(float64(addressed+fromHer) / float64(len(turns)))

	return clamp01(0.4*length + 0.3*crowd + 0.3*involvement)
}

// Participants lists the user ids who took part, so a memory can come back
// because of who is in the room.
func Participants(turns []Turn) []string {
	seen := make(map[string]bool, len(turns))
	out := make([]string, 0, len(turns))
	for _, t := range turns {
		if t.FromBot || t.UserID == "" || seen[t.UserID] {
			continue
		}
		seen[t.UserID] = true
		out = append(out, t.UserID)
	}
	return out
}

// Settled reports whether a conversation has paused long enough to be worth
// remembering as one thing.
//
// Summarising while people are still talking produces a memory of half an
// argument, and then a second memory of the other half when they finish.
func Settled(turns []Turn, now time.Time, quiet time.Duration) bool {
	if len(turns) == 0 {
		return false
	}
	return now.Sub(turns[len(turns)-1].At) >= quiet
}
