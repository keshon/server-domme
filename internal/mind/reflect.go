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
    {"person": their number, "who": "who they are, as she knows them now — a short paragraph", "between": "where things stand between her and them now — one or two sentences", "feeling": "how she feels about them, a few words", "works": "how talking with them goes best, from how it has actually gone — one line, and what she does with them that she would not do with anyone else", "stays": ["the few moments with them that stay with her — at most 5, each one sentence, first person; keep the old ones that still matter, replace the ones that do not"]}
  ],
  "done": [numbers of the things she meant to do that are done or no longer worth doing],
  "settled": [numbers of the feelings still with her that the day has settled, if any were listed],
  "threads": [
    {"text": "something she now means to do", "person": the number of who it is about, or 0, "hours": hours from now until it is worth doing}
  ]
}`

// reflectRules are how she looks back. Relationships move here rather than per
// message: this is where persistence in good faith can turn into a friend,
// which nothing in v1 could ever let happen.
const reflectRules = `How to look back:
- Be honest and specific: what actually happened, what she said, how people were with her, and what that means for her.
- Let people change in her eyes. Someone who kept at it in good faith deserves more warmth than a first impression gave them; someone who was unkind has earned some distance. Keep what is still true from before.
- Write only about people who appear in the day. Keep facts they told her.
- What works with someone comes from how the days with them actually went — what they answered, what fell flat, the register they meet her in. Not from what they are: their age, their gender or their role predict nothing, and she has been wrong guessing.
- Never guess at anyone's gender: unless they have said, or it is already written down, a person is "they". Someone who has corrected her is right, and what was written before them is wrong.
- What stays with her about someone is what she would still remember in a year: the moments that hit hardest, good or bad, marked "it stayed with her". Small talk does not stay.
- Keep what she said her own: if she said something, she said it.
- When something she started went unanswered, weigh it against how often anyone gets an answer in that room. Most messages in a quiet room go unanswered; that is the room, not her.
- Fold a feeling the day has settled into how things stand between her and the person, and list it as settled. One that still stands is left alone.
- How she has been lately is what actually happened here: the rooms, the people, what she did and did not do. She has no work or project of her own beyond this server, and does not write of one — "still working on whatever I'm working on" is a thing she made up about herself.
- Do not invent anything that is not in the day.`

// RoomRate is how often anyone got an answer in one channel on a day: the
// base rate a response to her is read against. See docs/persona-v3.md, H5.
type RoomRate struct {
	Channel    string
	Messages   int
	Unanswered int
}

// Reflect looks back on one day: writes its summary, rewrites how she sees
// herself and the people she spent it with, and settles what she means to
// do. It reports false when there was nothing in the day to reflect on.
// rooms are the day's base rates, read against what she started.
func (m *Mind) Reflect(ctx context.Context, guildID, guildName string, date, now time.Time, rooms []RoomRate) (bool, error) {
	// A conversation last touched on the day, or before it, is over by the
	// time she looks back on it: closed into the day so the summary has it.
	// Not today's, when she is asked to reflect early — that one may still
	// be going.
	if end := startOf(date).AddDate(0, 0, 1); !end.After(startOf(now)) {
		if _, err := m.Memory.CloseArcsBefore(guildID, end); err != nil {
			return false, err
		}
	}
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

	live := memory.Live(self.Feelings, now)
	msgs := m.reflectPrompt(guildName, day, self, people, open, rooms, live, now)
	reply, _, err := m.generate(ai.WithRaw(ai.WithTemperature(ctx, reflectTemperature)), msgs)
	if err != nil {
		return false, err
	}
	obj, ok := decodeObject(reply)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnreadable, clip(reply, 200))
	}
	return true, m.applyReflection(guildID, date, now, obj, people, open, live)
}

func (m *Mind) reflectPrompt(guildName string, day memory.Day, self memory.Self, people []memory.Person, open []memory.Thread, rooms []RoomRate, live []memory.Feeling, now time.Time) []ai.Message {
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
		user.WriteString("\nThe people in it, as she knew them before today:")
		for i, p := range people {
			fmt.Fprintf(&user, "\n\nPerson %d. %s", i+1, renderPerson(p, "", now))
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
	if len(live) > 0 {
		user.WriteString("\nWhat is still with her:")
		for i, f := range live {
			line := oneLine(f.What)
			if f.About != "" {
				line += " — about " + oneLine(f.About)
			}
			fmt.Fprintf(&user, "\n%d. %s (%s)", i+1, line, ago(now.Sub(f.At)))
		}
		user.WriteString("\n")
	}
	if len(rooms) > 0 {
		user.WriteString("\nHow the rooms were that day:")
		for _, r := range rooms {
			fmt.Fprintf(&user, "\n- in #%s, %d of %d messages got no reply from anyone", r.Channel, r.Unanswered, r.Messages)
		}
		user.WriteString("\n")
	}
	user.WriteString("\nWhat does she make of the day?")
	return []ai.Message{
		{Role: ai.RoleSystem, Content: sys.String()},
		{Role: ai.RoleUser, Content: user.String()},
	}
}

// applyReflection writes what reflection concluded.
//
// People are referred to by the number they were shown under, never by their
// Discord id: an id is a stable identifier for a real account and the model
// has no business seeing one. A number outside the list is ignored, so a
// hallucinated person cannot overwrite anyone's dossier.
func (m *Mind) applyReflection(guildID string, date, now time.Time, obj map[string]any, people []memory.Person, open []memory.Thread, live []memory.Feeling) error {
	// Everything reflection writes is her interpretation of the day.
	from := memory.OnDay(memory.Interpreted, date.Format("2006-01-02"))
	refuse := func(kind, reason string) { m.refuse(guildID, "", kind, reason) }

	if summary := str(obj, "summary"); summary != "" {
		if err := m.Memory.SetSummary(guildID, date, summary); err != nil {
			return err
		}
	}
	lately := str(obj, "lately")
	settled := map[int]bool{}
	for _, n := range numbers(obj, "settled") {
		if n < 1 || n > len(live) {
			refuse(proposalFeeling, "settled a feeling that does not exist")
			continue
		}
		settled[n] = true
	}
	if err := m.Memory.UpdateSelf(guildID, func(me *memory.Self) {
		if lately != "" {
			me.Lately = lately
		}
		me.Reflected = now
		var kept []memory.Feeling
		for _, f := range me.Feelings {
			gone := f.Strength(now) < memory.FeelingGone
			for n := range settled {
				if live[n-1].At.Equal(f.At) && live[n-1].What == f.What {
					gone = true
				}
			}
			if !gone {
				kept = append(kept, f)
			}
		}
		me.Feelings = kept
	}); err != nil {
		return err
	}

	shown := func(entry map[string]any) (memory.Person, bool) {
		n := int(num(entry, "person"))
		if n < 1 || n > len(people) {
			return memory.Person{}, false
		}
		return people[n-1], true
	}
	for _, entry := range objects(obj, "people") {
		p, ok := shown(entry)
		if !ok {
			refuse(proposalPerson, "no such person in the day")
			continue
		}
		who, between, feeling := str(entry, "who"), str(entry, "between"), str(entry, "feeling")
		works := str(entry, "works")
		stays, hasStays := texts(entry, "stays")
		err := m.Memory.UpdatePerson(guildID, p.ID, func(d *memory.Person) {
			if hasStays {
				d.Kept = keep(d.Kept, stays, date, from)
			}
			if d.Name == "" {
				d.Name = p.Name
			}
			if who != "" {
				if mayOverwrite(d.WhoFrom, memory.Interpreted) {
					d.Who, d.WhoFrom = who, from
				} else {
					refuse(proposalPerson, "who they are: would overwrite a stronger kind")
				}
			}
			if works != "" {
				d.Works, d.WorksFrom = clip(works, maxBetweenChars), from
			}
			if between != "" {
				if mayOverwrite(d.BetweenFrom, memory.Interpreted) {
					d.Between, d.BetweenFrom = between, from
				} else {
					refuse(proposalBetween, "would overwrite a stronger kind")
				}
			}
			if feeling != "" {
				d.Feeling, d.FeelingFrom = clip(feeling, maxToward), from
			}
		})
		if err != nil {
			return err
		}
	}
	for _, n := range numbers(obj, "done") {
		if n < 1 || n > len(open) {
			refuse(proposalThread, "closed a thread that does not exist")
			continue
		}
		if err := m.Memory.CloseThread(guildID, open[n-1].Key()); err != nil {
			return err
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
		t := memory.Thread{Due: due, Text: clip(text, maxLaterChars), Source: from}
		if n := int(num(entry, "person")); n != 0 {
			p, ok := shown(entry)
			if !ok {
				refuse(proposalThread, "about a person not in the day")
				continue
			}
			t.Person = memory.Ref{ID: p.ID, Name: p.Name}
		}
		if err := m.Memory.AddThread(guildID, t); err != nil {
			return err
		}
	}
	return nil
}

// keep is what stays with her about someone after reflecting: the model's
// list, in its order, with a moment she already kept keeping the day it
// happened rather than taking the day she last thought about it.
func keep(old []memory.Note, stays []string, date time.Time, from memory.Source) []memory.Note {
	var out []memory.Note
	for _, text := range stays {
		n := memory.Note{Day: date, Text: text, Source: from}
		for _, o := range old {
			if strings.EqualFold(oneLine(o.Text), oneLine(text)) {
				n.Day = o.Day
			}
		}
		out = append(out, n)
	}
	if len(out) > memory.MaxKept {
		out = out[:memory.MaxKept]
	}
	return out
}
