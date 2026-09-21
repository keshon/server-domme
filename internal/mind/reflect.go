package mind

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
)

// reflectShape is what reflection answers with.
const reflectShape = `Answer with one JSON object and nothing else:
{
  "summary": "what the day was, for her — what happened and what it meant, 2 to 4 sentences, first person",
  "lately": "her account of how she has been lately, rewritten to take this day in — first person, under 120 words",
  "people": [
    {"id": "their id", "who": "who they are, as she knows them now — a short paragraph", "between": "where things stand between her and them now — one or two sentences", "feeling": "how she feels about them, a few words"}
  ],
  "done": [numbers of the things she meant to do that are done or no longer worth doing],
  "threads": [
    {"text": "something she now means to do", "person_id": "who it is about, or empty", "hours": hours from now until it is worth doing}
  ]
}`

// reflectRules are how she looks back. Relationships move here rather than per
// message: this is where persistence in good faith can turn into a friend,
// which nothing in v1 could ever let happen.
const reflectRules = `How to look back:
- Be honest and specific: what actually happened, what she said, how people were with her, and what that means for her.
- Let people change in her eyes. Someone who kept at it in good faith deserves more warmth than a first impression gave them; someone who was unkind has earned some distance. Keep what is still true from before.
- Write only about people who appear in the day. Keep facts they told her.
- Keep what she said her own: if she said something, she said it.
- Do not invent anything that is not in the day.`

// Reflect looks back on one day: writes its summary, rewrites how she sees
// herself and the people she spent it with, and settles what she means to
// do. It reports false when there was nothing in the day to reflect on.
func (m *Mind) Reflect(ctx context.Context, guildID, guildName string, date, now time.Time) (bool, error) {
	day, err := m.Memory.Day(guildID, date)
	if err != nil || len(day.Moments) == 0 {
		return false, err
	}
	self, err := m.Memory.Self(guildID)
	if err != nil {
		return false, err
	}
	if self.Lately == "" && m.Character != nil {
		self.Lately = m.Character.Lately
	}

	var people []memory.Person
	seen := make(map[string]bool)
	for _, mo := range day.Moments {
		for _, ref := range mo.People {
			if ref.ID == "" || seen[ref.ID] {
				continue
			}
			seen[ref.ID] = true
			p, ok, err := m.Memory.Person(guildID, ref.ID)
			if err != nil {
				return false, err
			}
			if !ok {
				p = memory.Person{ID: ref.ID}
			}
			if p.Name == "" {
				p.Name = ref.Name
			}
			people = append(people, p)
		}
	}
	threads, err := m.Memory.Threads(guildID)
	if err != nil {
		return false, err
	}
	open := memory.Unfinished(threads)

	msgs := m.reflectPrompt(guildName, day, self, people, open, now)
	reply, _, err := m.generate(ai.WithRaw(ai.WithTemperature(ctx, reflectTemperature)), msgs)
	if err != nil {
		return false, err
	}
	obj, ok := decodeObject(reply)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnreadable, clip(reply, 200))
	}
	return true, m.applyReflection(guildID, date, now, obj, people, open)
}

func (m *Mind) reflectPrompt(guildName string, day memory.Day, self memory.Self, people []memory.Person, open []memory.Thread, now time.Time) []ai.Message {
	name := "her"
	if m.Character != nil {
		name = m.Character.Name
	}
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are the inner life of %s, a person on the Discord server %q — not an assistant, "+
		"not a bot. It is the end of the day, and she is looking back on it.", name, guildName)
	if m.Character != nil && m.Character.Persona != "" {
		sys.WriteString("\n\nWho she is:\n" + m.Character.Persona)
	}
	sys.WriteString("\n\n" + reflectRules + "\n\n" + reflectShape)

	var user strings.Builder
	if self.Lately != "" {
		user.WriteString("How she saw herself before today:\n" + self.Lately + "\n\n")
	}
	fmt.Fprintf(&user, "The day, %s:\n", day.Date.Format("Monday 2 January"))
	for _, mo := range day.Moments {
		user.WriteString("- " + renderMoment(mo, day.Date.Add(36*time.Hour)) + "\n")
	}
	if len(people) > 0 {
		user.WriteString("\nThe people in it, as she knew them before today (id in brackets):")
		for _, p := range people {
			fmt.Fprintf(&user, "\n\n[%s] %s", p.ID, renderPerson(p, "", now))
		}
		user.WriteString("\n")
	}
	if len(open) > 0 {
		user.WriteString("\nThings she meant to do:")
		for i, t := range open {
			fmt.Fprintf(&user, "\n%d. %s", i+1, renderThread(t, now))
		}
		user.WriteString("\n")
	}
	user.WriteString("\nWhat does she make of the day?")
	return []ai.Message{
		{Role: ai.RoleSystem, Content: sys.String()},
		{Role: ai.RoleUser, Content: user.String()},
	}
}

// applyReflection writes what reflection concluded. A person the model named
// who was not in the day is ignored: reflection rewrites only the people it
// was shown, so a hallucinated id cannot overwrite someone's dossier.
func (m *Mind) applyReflection(guildID string, date, now time.Time, obj map[string]any, people []memory.Person, open []memory.Thread) error {
	if summary := str(obj, "summary"); summary != "" {
		if err := m.Memory.SetSummary(guildID, date, summary); err != nil {
			return err
		}
	}
	lately := str(obj, "lately")
	if err := m.Memory.UpdateSelf(guildID, func(me *memory.Self) {
		if lately != "" {
			me.Lately = lately
		}
		me.Reflected = now
	}); err != nil {
		return err
	}

	shown := make(map[string]memory.Person, len(people))
	for _, p := range people {
		shown[p.ID] = p
	}
	for _, entry := range objects(obj, "people") {
		id := str(entry, "id")
		p, ok := shown[id]
		if !ok {
			continue
		}
		who, between, feeling := str(entry, "who"), str(entry, "between"), str(entry, "feeling")
		err := m.Memory.UpdatePerson(guildID, id, func(d *memory.Person) {
			if d.Name == "" {
				d.Name = p.Name
			}
			if who != "" {
				d.Who = who
			}
			if between != "" {
				d.Between = between
			}
			if feeling != "" {
				d.Feeling = feeling
			}
		})
		if err != nil {
			return err
		}
	}

	for _, n := range numbers(obj, "done") {
		if n >= 1 && n <= len(open) {
			if err := m.Memory.CloseThread(guildID, open[n-1].Key()); err != nil {
				return err
			}
		}
	}
	for _, entry := range objects(obj, "threads") {
		text := str(entry, "text")
		if text == "" {
			continue
		}
		hours := num(entry, "hours")
		due := now.Add(laterDefault)
		if hours > 0 {
			due = now.Add(min(time.Duration(hours*float64(time.Hour)), laterMax))
		}
		t := memory.Thread{Due: due, Text: text}
		if p, ok := shown[str(entry, "person_id")]; ok {
			t.Person = memory.Ref{ID: p.ID, Name: p.Name}
		}
		if err := m.Memory.AddThread(guildID, t); err != nil {
			return err
		}
	}
	return nil
}
