package mind

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Person is someone she could name in a message: who they are to Discord and
// what they are called in the conversation.
type Person struct {
	ID   string
	Name string
}

// ResolveMentions turns "@Name" in what she wrote into a real Discord mention
// for anyone in people, and reports whose ids it used.
//
// The model only ever writes text and never sees an id, so without this "@Big
// M" reaches the channel as nine grey characters. Only people in the
// conversation are resolved: a name she was never shown is left as she wrote
// it rather than guessed at.
//
// Longer names are tried first, so "@Big Mike" is not claimed by "Big M", and
// a name only matches when it ends there — "@Big Mama" is nobody's mention.
// Matching ignores case, since she writes in lowercase.
//
// Whether anyone is actually notified is decided separately, when the message
// is sent; see chat.Service.send.
func ResolveMentions(text string, people []Person) (string, []string) {
	if !strings.Contains(text, "@") || len(people) == 0 {
		return text, nil
	}

	candidates := make([]Person, 0, len(people))
	for _, p := range people {
		if p.ID != "" && strings.TrimSpace(p.Name) != "" {
			candidates = append(candidates, p)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return len(candidates[i].Name) > len(candidates[j].Name)
	})

	var out strings.Builder
	var used []string
	seen := make(map[string]bool)

	for i := 0; i < len(text); {
		if text[i] != '@' {
			out.WriteByte(text[i])
			i++
			continue
		}
		rest := text[i+1:]
		matched := false
		for _, p := range candidates {
			if !hasNamePrefix(rest, p.Name) {
				continue
			}
			out.WriteString("<@" + p.ID + ">")
			i += 1 + len(p.Name)
			if !seen[p.ID] {
				seen[p.ID] = true
				used = append(used, p.ID)
			}
			matched = true
			break
		}
		if !matched {
			out.WriteByte('@')
			i++
		}
	}
	return out.String(), used
}

// hasNamePrefix reports whether s opens with name, ignoring case, and the name
// ends there rather than running on into a longer word.
func hasNamePrefix(s, name string) bool {
	if len(s) < len(name) || !strings.EqualFold(s[:len(name)], name) {
		return false
	}
	next, _ := utf8.DecodeRuneInString(s[len(name):])
	return next == utf8.RuneError || !unicode.IsLetter(next) && !unicode.IsDigit(next)
}
