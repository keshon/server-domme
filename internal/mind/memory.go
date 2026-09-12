package mind

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Memory is one thing she remembers happening in a channel.
//
// Written once and never rewritten. The obvious alternative — re-summarising a
// memory shorter as it ages, so the stored text shrinks — costs a model call
// per memory per age bracket and drifts: summarising a summary pulls towards
// the blandest available reading every time, which is visible in the cognitum
// experiment's logs as the same observation reappearing in three slightly
// different wordings. Storing it once and rendering less of it gives the same
// gradient for one call, and what comes back is what went in.
type Memory struct {
	At time.Time
	// Gist is the one-clause version, all that survives into old age.
	Gist string
	// Detail is the fuller account, shown only while the memory is bright.
	Detail string
	// Weight is how much the moment mattered, 0..1. It slows decay rather
	// than raising brightness: a charged memory is not more present than a
	// fresh one, it simply outlasts it.
	Weight float64
	// People are the user ids who were there, which is what lets a memory
	// come back because of who is in the room rather than what is being said.
	People []string
}

// Recall tuning.
const (
	// memoryHalflife is how long an unremarkable memory takes to halve in
	// brightness. Days rather than hours: the conversation buffer already
	// covers the last half hour, and anything this holds is by definition
	// something the channel has moved on from.
	memoryHalflife = 48 * time.Hour
	// weightedHalflifeGain is how much a fully weighted memory extends its own
	// halflife. Five is chosen so a charged day still registers a fortnight
	// later while ordinary chatter from the same day has gone.
	weightedHalflifeGain = 5
	// brightnessFloor is the point below which a memory is not worth the
	// tokens. Taken from the cognitum spec, which used the same value for the
	// same purpose.
	brightnessFloor = 0.05

	// topicBoost is added when the memory is about what is being discussed
	// now, and presenceBoost when someone who was there is here again. These
	// are what make an old memory surface at the right moment rather than
	// simply fading on a timer — cognitum's P6, "old thoughts can return when
	// a trigger matches their topic".
	topicBoost    = 0.35
	presenceBoost = 0.20
)

// Brightness is how strongly a memory comes back, 0..1.
//
// Age decides the floor it decays towards; the room decides whether it
// surfaces now. Both matter and they are different questions: how likely a
// thing is to be recalled at all, and how much of it comes back.
func (m Memory) Brightness(now time.Time, topic []string, present []string) float64 {
	age := now.Sub(m.At)
	if age < 0 {
		age = 0
	}

	halflife := float64(memoryHalflife) * (1 + clamp01(m.Weight)*weightedHalflifeGain)
	bright := math.Pow(0.5, float64(age)/halflife)

	if overlap := wordOverlap(topic, Keywords(m.Gist+" "+m.Detail)); overlap > 0 {
		bright += topicBoost * overlap
	}
	if anyPresent(m.People, present) {
		bright += presenceBoost
	}
	return clamp01(bright)
}

// Recall picks the memories worth putting in front of her, brightest first.
func Recall(memories []Memory, now time.Time, topic, present []string, max int) []Memory {
	type scored struct {
		m Memory
		b float64
	}

	lit := make([]scored, 0, len(memories))
	for _, m := range memories {
		if strings.TrimSpace(m.Gist) == "" {
			continue
		}
		if b := m.Brightness(now, topic, present); b >= brightnessFloor {
			lit = append(lit, scored{m: m, b: b})
		}
	}

	sort.SliceStable(lit, func(i, j int) bool { return lit[i].b > lit[j].b })
	if max > 0 && len(lit) > max {
		lit = lit[:max]
	}

	out := make([]Memory, 0, len(lit))
	for _, s := range lit {
		out = append(out, s.m)
	}
	return out
}

// Render writes recalled memories the way they are remembered: fully while
// they are bright, and eventually as no more than a hint of what it was.
func Render(memories []Memory, now time.Time, topic, present []string) string {
	if len(memories) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Things you remember happening here:\n")
	for _, m := range memories {
		b.WriteString("- ")
		b.WriteString(m.recollection(now, topic, present))
		b.WriteString("\n")
	}
	return b.String()
}

// Detail thresholds. Three bands rather than a smooth fade, because the
// rendering is sentences and a sentence is either there or it is not.
const (
	vividAbove   = 0.60
	clearedAbove = 0.30
	// clippedDetail is how much of Detail survives the middle band.
	clippedDetail = 90
)

func (m Memory) recollection(now time.Time, topic, present []string) string {
	gist := strings.TrimSpace(m.Gist)
	detail := strings.TrimSpace(m.Detail)
	when := roughDuration(now.Sub(m.At))

	switch b := m.Brightness(now, topic, present); {
	case b > vividAbove && detail != "":
		return gist + " (" + when + " ago) — " + detail
	case b > clearedAbove && detail != "":
		return gist + " (" + when + " ago) — " + trimTo(detail, clippedDetail)
	default:
		// Old enough that only the shape of it is left. No timestamp either:
		// someone who barely remembers a thing does not know exactly when.
		return gist
	}
}

// stopWords are too common to say anything about what a message is about.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"is": true, "was": true, "are": true, "were": true, "be": true, "been": true,
	"to": true, "of": true, "in": true, "on": true, "at": true, "for": true,
	"it": true, "its": true, "this": true, "that": true, "with": true,
	"i": true, "you": true, "he": true, "she": true, "we": true, "they": true,
	"not": true, "no": true, "yes": true, "do": true, "did": true, "does": true,
	"what": true, "who": true, "how": true, "why": true, "when": true,
	"about": true, "from": true, "just": true, "all": true, "some": true,
	"there": true, "here": true, "then": true, "than": true, "into": true,
	"have": true, "has": true, "had": true, "will": true, "would": true,
	"и": true, "в": true, "на": true, "не": true, "что": true, "это": true,
	"как": true, "он": true, "она": true, "они": true, "мы": true, "вы": true,
}

// minKeyword is the shortest run of letters worth treating as a subject.
const minKeyword = 3

// Keywords reduces text to the words that say what it was about.
//
// Set overlap rather than embeddings, which would need a model call per
// memory per message and a vector store to put them in. The cost is that it
// matches words rather than meanings, so a memory about "the purge rules"
// will not surface for "channel cleanup". That is a worse recall than a real
// one and a much better one than none.
func Keywords(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !isAlphanumeric(r)
	})

	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len([]rune(f)) < minKeyword || stopWords[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// wordOverlap is the share of the memory's keywords that the current topic
// also uses, 0..1.
func wordOverlap(topic, memory []string) float64 {
	if len(topic) == 0 || len(memory) == 0 {
		return 0
	}
	inTopic := make(map[string]bool, len(topic))
	for _, w := range topic {
		inTopic[w] = true
	}

	var hits int
	for _, w := range memory {
		if inTopic[w] {
			hits++
		}
	}
	return float64(hits) / float64(len(memory))
}

func anyPresent(were, are []string) bool {
	if len(were) == 0 || len(are) == 0 {
		return false
	}
	here := make(map[string]bool, len(are))
	for _, id := range are {
		here[id] = true
	}
	for _, id := range were {
		if here[id] {
			return true
		}
	}
	return false
}
