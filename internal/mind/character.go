package mind

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Character section headings. These are the contract between the authored
// character file and this parser: renaming one silently drops that section,
// so they are matched case-insensitively and nothing else is treated as a
// section boundary.
const (
	headingExamples = "examples"
	headingAvoid    = "avoid"
	// headingNotes is parsed and discarded. Everything else in the file is
	// sent to the model on every single message, so without somewhere to put
	// guidance for whoever edits the character next, that guidance either
	// goes missing or gets paid for a few hundred times a day.
	headingNotes = "notes"
	// headingLately is who she is lately, in her own words, before she has
	// reflected on anything. It seeds self.md the first time she speaks in a
	// guild and is not sent as part of the persona after that; see
	// memory.Self.
	headingLately = "lately"
	// headingSpecifics is a list of concrete facts about her: opinions with a
	// reason, tastes, things she is bad at, words she overuses. Kept apart
	// from the persona so they can be chosen by what the conversation is
	// about when there are too many to send. See docs/persona-v3.md, F1.
	headingSpecifics = "specifics"
	// headingNotHers is a list of phrases she never uses: the tics a model
	// brings to every character — "the real question is", "honestly". A
	// reply containing one is asked for again; see Style.
	headingNotHers = "not her words"
)

// Example speaker labels inside the examples section.
const (
	labelUser = "user:"
	labelHer  = "her:"
)

// Exchange is one authored example of the character speaking.
//
// These are the highest-value part of the whole character file. They are
// replayed to the model as real conversation turns rather than quoted inside
// the system prompt, because a model imitates a voice it has seen far more
// reliably than one it has been described. Do not "simplify" them into a
// bulleted style guide — that is precisely the change that made the previous
// version sound like every other assistant.
//
// Situations are what kinds of moment it answers, from the "### jab"
// headings above it in the examples section; none means any. The voice is
// shown mostly examples of the situation in front of her. See Situation.
type Exchange struct {
	User       string
	Assistant  string
	Situations []Situation
}

// Character is the authored identity: who she is, in prose, plus examples of
// her speaking and a list of things she does not do.
type Character struct {
	// Name is what she is called. Used to strip her own label out of replies
	// and to address her in the prompt.
	Name string
	// Persona is the prose identity, passed through verbatim and never
	// trimmed. It is authored text, so a budget that cut it would be cutting
	// the one part of the prompt a human deliberately wrote.
	Persona string
	// Avoid lists hard limits, kept separate from Persona so they can be
	// stated last in the system prompt, where instructions hold best.
	Avoid []string
	// Examples are replayed as conversation turns. See Exchange.
	Examples []Exchange
	// Lately seeds how she sees herself; see headingLately.
	Lately string
	// Specifics are the concrete facts the author wrote about her; see
	// headingSpecifics. They are the one kind of memory marked authored,
	// and nothing she says overwrites them.
	Specifics []string
	// NotHers are phrases she never uses; see headingNotHers.
	NotHers []string
	// UnknownSituations are example headings that name no situation: a
	// typo, which files its examples under nothing. Reported, not fatal.
	UnknownSituations []string
}

// LoadCharacter reads a character file from path.
func LoadCharacter(name, path string) (*Character, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("mind: open character file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Returned unwrapped: ParseCharacter names itself, and the caller already
	// logs the path it was handed. Wrapping here would print the prefix twice.
	return ParseCharacter(name, f)
}

// ParseCharacter reads a character definition.
//
// The format is Markdown so the file stays pleasant to write by hand: prose
// under any heading becomes part of the persona, "## Avoid" becomes the limits
// list, "## Specifics" a list of concrete facts about her, "## Examples"
// holds alternating "user:" and "her:" lines, and "## Notes" is dropped. An
// unrecognised heading is persona rather than an error — the file is
// authored content, and failing to start the bot over a typo in a heading
// serves nobody.
func ParseCharacter(name string, r io.Reader) (*Character, error) {
	c := &Character{Name: name}
	var lately strings.Builder

	var persona strings.Builder
	var pending Exchange
	var situations []Situation
	section := ""

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if heading, ok := sectionHeading(trimmed); ok {
			// A half-built exchange belongs to the section that is ending.
			pending = flushExchange(c, pending)
			if section == headingExamples && headingLevel(trimmed) >= 3 {
				// A subheading of the examples files what follows under
				// situations, and is not a section of its own.
				situations = c.readSituations(heading)
				continue
			}
			section, situations = heading, nil
			continue
		}

		switch section {
		case headingExamples:
			pending = readExampleLine(c, pending, trimmed)
			pending.Situations = situations
		case headingAvoid:
			if item := listItem(trimmed); item != "" {
				c.Avoid = append(c.Avoid, item)
			}
		case headingSpecifics:
			if item := listItem(trimmed); item != "" {
				c.Specifics = append(c.Specifics, item)
			}
		case headingNotHers:
			if item := listItem(trimmed); item != "" {
				c.NotHers = append(c.NotHers, item)
			}
		case headingLately:
			lately.WriteString(line)
			lately.WriteString("\n")
		case headingNotes:
			// Deliberately dropped; see headingNotes.
		default:
			persona.WriteString(line)
			persona.WriteString("\n")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("mind: read character file: %w", err)
	}
	flushExchange(c, pending)

	c.Persona = strings.TrimSpace(persona.String())
	c.Lately = strings.TrimSpace(lately.String())
	if c.Persona == "" && len(c.Examples) == 0 {
		return nil, fmt.Errorf("mind: character file has neither persona text nor examples")
	}
	return c, nil
}

// sectionHeading reports the lowercased name of a Markdown heading.
func sectionHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "# "))), true
}

// headingLevel is how many #s a heading starts with.
func headingLevel(line string) int {
	return len(line) - len(strings.TrimLeft(line, "#"))
}

// readSituations reads an examples subheading: one situation or several,
// separated by commas or slashes. A name that is not a situation is noted
// in UnknownSituations and files nothing.
func (c *Character) readSituations(heading string) []Situation {
	var out []Situation
	for _, part := range strings.FieldsFunc(heading, func(r rune) bool { return r == ',' || r == '/' }) {
		if s := ParseSituation(part); s != "" {
			out = append(out, s)
		} else if part = strings.TrimSpace(part); part != "" {
			c.UnknownSituations = append(c.UnknownSituations, part)
		}
	}
	return out
}

// listItem returns the text of a Markdown list item, or "" for anything else.
func listItem(line string) string {
	for _, marker := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, marker) {
			return strings.TrimSpace(strings.TrimPrefix(line, marker))
		}
	}
	return ""
}

// readExampleLine folds one line of the examples section into pending.
func readExampleLine(c *Character, pending Exchange, line string) Exchange {
	line = strings.TrimPrefix(line, ">")
	line = strings.TrimSpace(line)
	if item := listItem(line); item != "" {
		line = item
	}

	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, labelUser):
		// A new prompt starts a new exchange; whatever came before is done.
		pending = flushExchange(c, pending)
		pending.User = strings.TrimSpace(line[len(labelUser):])
	case strings.HasPrefix(lower, labelHer):
		pending.Assistant = strings.TrimSpace(line[len(labelHer):])
	}
	return pending
}

// flushExchange stores pending if it is complete and returns an empty one.
// An exchange missing either half is dropped: a prompt with no reply teaches
// the model nothing, and a reply with no prompt teaches it to speak unbidden.
func flushExchange(c *Character, pending Exchange) Exchange {
	if pending.User != "" && pending.Assistant != "" {
		c.Examples = append(c.Examples, pending)
	}
	return Exchange{}
}
