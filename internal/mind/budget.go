package mind

import (
	"sort"

	"github.com/keshon/server-domme/internal/ai"
)

// Prompt budgets, in characters. Each piece of what she knows is bounded on
// its own, but nothing bounded the sum, and the free relays refuse or time
// out on long prompts. Over budget, what she knows gives way in a fixed
// order — see fit — and never the persona, the live conversation or the
// person being answered. See docs/persona-v3.md, Known failure points.
const (
	thinkBudget = 16000
	voiceBudget = 12000
)

// Floors the trimming stops at before moving to the next thing.
const (
	minSelfFacts = 3
	minSpecifics = 10
)

// fit trims k until the prompt size reports fits budget, giving way in
// order: recalled moments, weakest first; self-facts beyond the best
// matches; specifics beyond the first few; dossier notes, oldest first. It
// reports what it trimmed in the log.
func (m *Mind) fit(guildID, prompt string, k Known, budget int, size func(Known) int) Known {
	// Every call reports its size, trimmed or not: what breaks first here is
	// not a refused call but a prompt quietly giving way, and a number in
	// the log each time shows it coming. Two days in, one person, the
	// thinking prompt was at nine tenths of its budget.
	before := size(k)
	m.Log.Debug().
		Str("guild_id", guildID).
		Str("prompt", prompt).
		Int("size", before).
		Int("budget", budget).
		Msg("mind_prompt_sized")
	if before <= budget {
		return k
	}
	trimmed := map[string]int{}
	over := func() bool { return size(k) > budget }

	k.Recalled = append(k.Recalled[:0:0], k.Recalled...)
	sort.SliceStable(k.Recalled, func(i, j int) bool { return k.Recalled[i].Score > k.Recalled[j].Score })
	for over() && len(k.Recalled) > 0 {
		k.Recalled = k.Recalled[:len(k.Recalled)-1]
		trimmed["recalled"]++
	}
	sort.SliceStable(k.Recalled, func(i, j int) bool { return k.Recalled[i].At.Before(k.Recalled[j].At) })

	for over() && len(k.SelfFacts) > minSelfFacts {
		k.SelfFacts = k.SelfFacts[:len(k.SelfFacts)-1]
		trimmed["self-facts"]++
	}
	for over() && len(k.Specifics) > minSpecifics {
		k.Specifics = k.Specifics[:len(k.Specifics)-1]
		trimmed["specifics"]++
	}

	// People are copied before their notes are cut: k shares them with the
	// caller.
	people := append(k.People[:0:0], k.People...)
	k.People = people
	for over() {
		cut := false
		for i := range k.People {
			if len(k.People[i].Notes) > 0 {
				k.People[i].Notes = k.People[i].Notes[1:]
				trimmed["notes"]++
				cut = true
				if !over() {
					break
				}
			}
		}
		if !cut {
			break
		}
	}

	ev := m.Log.Info().Str("guild_id", guildID).Str("prompt", prompt).Int("budget", budget).
		Int("was", before).Int("size", size(k))
	for what, n := range trimmed {
		ev = ev.Int(what, n)
	}
	ev.Msg("mind_prompt_trimmed")
	return k
}

// promptSize is the characters in a prompt.
func promptSize(msgs []ai.Message) int {
	n := 0
	for _, msg := range msgs {
		n += len(msg.Content)
	}
	return n
}
