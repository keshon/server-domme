package mind

import (
	"fmt"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Render limits, in characters. The dossier and memory are prose the model
// reads on every call; these stop a file someone let grow from crowding out
// the conversation it is meant to inform.
const (
	maxLatelyChars  = 900
	maxWhoChars     = 600
	maxBetweenChars = 500
	maxNotesShown   = 6
	maxMomentChars  = 300
)

// renderWorld is what she knows, as the thinking prompts show it: where and
// when she is, how she has been, who is here and what she remembers of them,
// and what she means to do.
func renderWorld(s Scene, k Known) string {
	var b strings.Builder

	b.WriteString(renderPlace(s))

	if lately := clip(k.Self.Lately, maxLatelyChars); lately != "" {
		b.WriteString("\n\nHow she has been lately, in her own words:\n" + lately)
	}
	if mood := moodLine(k.Self, s.Now); mood != "" {
		b.WriteString("\n\n" + mood)
	}

	if len(k.Days) > 0 {
		b.WriteString("\n\nThe last few days, as she remembers them:")
		for _, d := range k.Days {
			fmt.Fprintf(&b, "\n- %s: %s", d.Date.Format("Monday 2 Jan"), oneLine(d.Summary))
		}
	}

	if len(k.People) > 0 {
		b.WriteString("\n\nPeople in this:")
		for _, p := range k.People {
			b.WriteString("\n\n" + renderPerson(p, s.Roles[p.ID], s.Now))
		}
	}

	if len(k.Recalled) > 0 {
		b.WriteString("\n\nWhat she remembers that may bear on this:")
		for _, m := range k.Recalled {
			b.WriteString("\n- " + renderMoment(m, s.Now))
		}
	}

	if len(k.Threads) > 0 {
		b.WriteString("\n\nThings she means to do:")
		for i, t := range k.Threads {
			fmt.Fprintf(&b, "\n%d. %s", i+1, renderThread(t, s.Now))
		}
	}
	return b.String()
}

// renderPlace is where and when.
func renderPlace(s Scene) string {
	now := s.Now
	var b strings.Builder
	fmt.Fprintf(&b, "It is %s, %s %s.", now.Format("Monday 2 January"), partOfDay(now), now.Format("15:04"))
	switch {
	case s.ChannelName != "" && s.GuildName != "":
		fmt.Fprintf(&b, " She is in #%s on the Discord server %q.", s.ChannelName, s.GuildName)
	case s.GuildName != "":
		fmt.Fprintf(&b, " She is on the Discord server %q.", s.GuildName)
	}
	if topic := strings.TrimSpace(s.ChannelTopic); topic != "" {
		b.WriteString(" The channel's topic: " + oneLine(topic))
	}
	if brief := strings.TrimSpace(s.Brief); brief != "" {
		b.WriteString("\nWhat the server is: " + oneLine(brief))
	}
	return b.String()
}

// moodLine states her mood with its age, since a mood from this morning is
// not a mood now; the model is trusted to let an old one fade.
func moodLine(self memory.Self, now time.Time) string {
	if self.Mood == "" {
		return ""
	}
	if self.MoodAt.IsZero() {
		return "Her mood: " + self.Mood + "."
	}
	return fmt.Sprintf("Her mood, as of %s: %s.", ago(now.Sub(self.MoodAt)), self.Mood)
}

// renderPerson is one dossier.
func renderPerson(p memory.Person, role string, now time.Time) string {
	var b strings.Builder
	name := p.Name
	if name == "" {
		name = "someone"
	}
	b.WriteString("### " + name)
	if role = strings.TrimSpace(role); role != "" {
		b.WriteString("\nWhat the server says about them: " + oneLine(role))
	}
	if p.Who == "" && p.Between == "" && len(p.Notes) == 0 && p.LastTalked.IsZero() {
		b.WriteString("\nShe does not know them yet.")
		return b.String()
	}
	if who := clip(p.Who, maxWhoChars); who != "" {
		b.WriteString("\nWho they are: " + who)
	}
	if between := clip(p.Between, maxBetweenChars); between != "" {
		b.WriteString("\nBetween them: " + between)
	}
	if p.Feeling != "" {
		b.WriteString("\nHow she feels about them: " + p.Feeling)
	}
	notes := p.Notes
	if len(notes) > maxNotesShown {
		notes = notes[len(notes)-maxNotesShown:]
	}
	for _, n := range notes {
		if n.Day.IsZero() {
			b.WriteString("\n- " + oneLine(n.Text))
		} else {
			b.WriteString("\n- (" + ago(now.Sub(n.Day)) + ") " + oneLine(n.Text))
		}
	}
	if !p.LastTalked.IsZero() {
		b.WriteString("\nLast talked with her: " + ago(now.Sub(p.LastTalked)) + ".")
	}
	return b.String()
}

// renderMoment is one remembered moment.
func renderMoment(m memory.Moment, now time.Time) string {
	var where []string
	if m.Channel != "" {
		where = append(where, "in #"+m.Channel)
	}
	var who []string
	for _, p := range m.People {
		if p.Name != "" {
			who = append(who, p.Name)
		}
	}
	if len(who) > 0 {
		where = append(where, "with "+strings.Join(who, ", "))
	}
	head := when(m.At, now)
	if len(where) > 0 {
		head += " (" + strings.Join(where, ", ") + ")"
	}
	return head + ": " + clip(m.Text, maxMomentChars)
}

// renderThread is one intention.
func renderThread(t memory.Thread, now time.Time) string {
	line := oneLine(t.Text)
	if t.Person.Name != "" {
		line += " [about " + t.Person.Name + "]"
	}
	return line + " — " + dueIn(t.Due, now)
}

// renderTranscript is the live conversation as the thinking prompts show it:
// clock times, names, and her own lines marked as hers. Marked "you" rather
// than with her name, because the whole failure this exists to prevent is her
// reading her own words as somebody else's.
func renderTranscript(turns []Turn, now time.Time) string {
	if len(turns) == 0 {
		return "(nothing has been said here recently)"
	}
	var b strings.Builder
	for _, t := range turns {
		if strings.TrimSpace(t.Content) == "" {
			continue
		}
		who := t.Username
		if t.FromBot {
			who = "YOU (she said this)"
		}
		if who == "" {
			who = "someone"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", when(t.At, now), who, t.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}

// clip trims text to a rune budget at a sentence or word boundary.
func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndexAny(cut, ".!?\n"); i > max/2 {
		return strings.TrimSpace(cut[:i+1])
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}

// oneLine flattens text into a single line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
