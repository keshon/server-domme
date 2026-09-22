package mind

import (
	"sort"
	"strings"

	"github.com/keshon/server-domme/internal/memory"
)

// Identity content: the author's specifics and what she has said about
// herself. Both are what give her something to say that is hers rather than
// the archetype's; see docs/persona-v3.md, workstream F.

// specificsBudget is how many characters of specifics go in front of her.
// Under it, every one is sent: they are the most valuable authored text
// after the persona. Over it, the ones the conversation touches come first.
const specificsBudget = 2400

// maxSelfFactsShown is how many of her self-facts are shown at once, and
// only ones that bear on the conversation. Kept is not the same as in mind.
const maxSelfFactsShown = 8

// pickSpecifics chooses the specifics for a conversation about words.
func pickSpecifics(all []string, words []string) []string {
	total := 0
	for _, s := range all {
		total += len(s)
	}
	if total <= specificsBudget {
		return all
	}
	order := byMatch(len(all), func(i int) string { return all[i] }, words)
	var out []int
	used := 0
	for _, i := range order {
		if used+len(all[i]) > specificsBudget {
			continue
		}
		used += len(all[i])
		out = append(out, i)
	}
	// Back in the author's order: it was written to be read that way.
	sort.Ints(out)
	picked := make([]string, 0, len(out))
	for _, i := range out {
		picked = append(picked, all[i])
	}
	return picked
}

// pickSelfFacts chooses what she has said about herself that bears on a
// conversation about words: only facts that share a word with it, best
// match first, at most maxSelfFactsShown.
func pickSelfFacts(all []memory.SelfFact, words []string) []memory.SelfFact {
	want := wordSet(words)
	type scored struct {
		f    memory.SelfFact
		hits int
	}
	var matched []scored
	for _, f := range all {
		if f.Text == "" || !f.Superseded.IsZero() {
			continue
		}
		if hits := overlapCount(memory.Keywords(f.Text), want); hits > 0 {
			matched = append(matched, scored{f, hits})
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].hits > matched[j].hits })
	if len(matched) > maxSelfFactsShown {
		matched = matched[:maxSelfFactsShown]
	}
	out := make([]memory.SelfFact, 0, len(matched))
	for _, m := range matched {
		out = append(out, m.f)
	}
	return out
}

// byMatch orders n items by how many of words each shares, most first,
// keeping the given order among equals.
func byMatch(n int, text func(int) string, words []string) []int {
	want := wordSet(words)
	hits := make([]int, n)
	order := make([]int, n)
	for i := range order {
		order[i] = i
		hits[i] = overlapCount(memory.Keywords(text(i)), want)
	}
	sort.SliceStable(order, func(a, b int) bool { return hits[order[a]] > hits[order[b]] })
	return order
}

func wordSet(words []string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

func overlapCount(words []string, want map[string]bool) int {
	n := 0
	for _, w := range words {
		if want[w] {
			n++
		}
	}
	return n
}

// renderSpecifics is the specifics as a list under a heading, or "".
func renderSpecifics(heading string, specifics []string) string {
	if len(specifics) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(heading)
	for _, s := range specifics {
		b.WriteString("\n- " + oneLine(s))
	}
	return b.String()
}

// renderSelfFacts is what she has said about herself, as a list under a
// heading, or "". The source never reaches the model: a message reference
// carries a Discord id.
func renderSelfFacts(heading string, facts []memory.SelfFact) string {
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(heading)
	for _, f := range facts {
		b.WriteString("\n- " + oneLine(f.Text))
	}
	return b.String()
}
