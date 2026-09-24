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
	if body := renderBody(s, "She", "has"); body != "" {
		b.WriteString(" " + body)
	}
	if r := renderReactions(s.Reactions, "her"); r != "" {
		b.WriteString("\n" + r)
	}

	if lately := clip(k.Self.Lately, maxLatelyChars); lately != "" {
		b.WriteString("\n\nHow she has been lately, in her own words:\n" + lately)
	}
	if mood := moodLine(k.Self, s.Now); mood != "" {
		b.WriteString("\n\n" + mood)
	}
	if feelings := renderFeelings(k.Self.Feelings, s.Now, "her"); feelings != "" {
		b.WriteString("\n\n" + feelings)
	}
	if drives := renderDrives(s, k); drives != "" {
		b.WriteString("\n\n" + drives)
	}
	if onMind := renderOnMind(k.Self, s.Now, "her"); onMind != "" {
		b.WriteString("\n\n" + onMind)
	}
	if life := renderLife(freshLife(k.Self.Life, s.Now)); life != "" {
		b.WriteString("\n\n" + life)
	}
	if wants := renderWants(freshWants(k.Self.Wants, s.Now)); wants != "" {
		b.WriteString("\n\n" + wants)
	}

	if len(k.Days) > 0 {
		b.WriteString("\n\nThe last few days, as she remembers them:")
		for _, d := range k.Days {
			fmt.Fprintf(&b, "\n- %s: %s", d.Date.Format("Monday 2 Jan"), oneLine(d.Summary))
		}
	}

	if facts := renderSelfFacts("Things she has said about herself that bear on this:", k.SelfFacts); facts != "" {
		b.WriteString("\n\n" + facts)
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

	if len(s.Reads) > 0 {
		b.WriteString("\n\nRooms she passes through and never speaks in: ")
		for i, c := range s.Reads {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("#" + c)
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

// renderBody is what her body is doing, as facts: when she woke, and how
// long she has been talking here. subject and has are "She"/"has" or
// "You"/"have", for the thinking and the voice.
func renderBody(s Scene, subject, has string) string {
	var parts []string
	if !s.Woke.IsZero() && s.Now.Sub(s.Woke) < 20*time.Hour {
		woke := s.Woke.In(s.Now.Location()).Format("15:04")
		if s.WokenEarly {
			parts = append(parts, fmt.Sprintf("%s was woken early, at %s.", subject, woke))
		} else {
			parts = append(parts, fmt.Sprintf("%s woke at %s.", subject, woke))
		}
	}
	if !s.Dropped.IsZero() {
		them := "her"
		if subject == "You" {
			them = "you"
		}
		parts = append(parts, fmt.Sprintf("They asked %s to drop something %s, and it has not come up since.",
			them, ago(s.Now.Sub(s.Dropped))))
	}
	if s.TalkingFor >= talkingWorthSaying {
		line := fmt.Sprintf("%s %s been talking here for %s", subject, has, gap(s.TalkingFor))
		if s.TalkingWith > 1 {
			line += fmt.Sprintf(", with %d people", s.TalkingWith)
		}
		parts = append(parts, line+".")
	}
	return strings.Join(parts, " ")
}

// renderReactions is what people put on her messages here since she last
// spoke: "Since her last message here: ❤️ ×2 from Big M and Ava, 😂 from
// Rook." whose is "her" or "your".
func renderReactions(rs []Reaction, whose string) string {
	if len(rs) == 0 {
		return ""
	}
	var parts []string
	for _, r := range rs {
		part := r.Emoji
		if r.Count > 1 {
			part += fmt.Sprintf(" ×%d", r.Count)
		}
		if len(r.Names) > 0 {
			part += " from " + joinNames(r.Names)
		}
		parts = append(parts, part)
	}
	return "Since " + whose + " last message here, people reacted to it: " + strings.Join(parts, ", ") + "."
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// renderFeelings is what is still with her, most recent first, each with
// its age: "stung — about Big M's jab (2 hours ago)". All of them, not the
// strongest: a person is annoyed with one friend and excited about a thing
// at once, and the order must not say which one should win.
func renderFeelings(feelings []memory.Feeling, now time.Time, whom string) string {
	live := memory.Live(feelings, now)
	if len(live) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Still with " + whom + ":")
	for _, f := range live {
		line := oneLine(f.What)
		if f.About != "" {
			line += " — about " + oneLine(f.About)
		}
		b.WriteString("\n- " + line + " (" + ago(now.Sub(f.At)) + ")")
	}
	return b.String()
}

// Drive facts are stated only past these: shorter is just a day.
const (
	quietWorthSaying = 2 * time.Hour
	heavyWorthSaying = 24 * time.Hour
)

// renderDrives are the facts a need is formed from — how long nobody has
// talked to her, how long since anything weighed on her — stated as facts.
// "She is bored" is the model's reading of them, never the code's.
func renderDrives(s Scene, k Known) string {
	var parts []string
	if s.QuietFor >= quietWorthSaying {
		parts = append(parts, fmt.Sprintf("Nobody has spoken to her here for %s.", gap(s.QuietFor)))
	}
	if !k.LastHeavy.IsZero() && s.Now.Sub(k.LastHeavy) >= heavyWorthSaying {
		parts = append(parts, fmt.Sprintf("The last thing that weighed on her was %s.", ago(s.Now.Sub(k.LastHeavy))))
	}
	return strings.Join(parts, " ")
}

// talkingWorthSaying is how long a conversation has to have gone on before
// its length is stated.
const talkingWorthSaying = 30 * time.Minute

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
	if p.Pronouns != "" {
		b.WriteString(" (" + oneLine(p.Pronouns) + ")")
	}
	if role = strings.TrimSpace(role); role != "" {
		b.WriteString("\nWhat the server says about them: " + oneLine(role))
	}
	if p.Who == "" && p.Between == "" && len(p.Kept) == 0 && len(p.Notes) == 0 && p.LastTalked.IsZero() {
		b.WriteString("\nShe does not know them yet.")
		return b.String()
	}
	if who := clip(p.Who, maxWhoChars); who != "" {
		b.WriteString("\nWho they are: " + who)
	}
	if between := clip(p.Between, maxBetweenChars); between != "" {
		b.WriteString("\nBetween them: " + between)
	}
	if works := clip(p.Works, maxBetweenChars); works != "" {
		b.WriteString("\nWhat works with them: " + works)
	}
	if p.Feeling != "" {
		b.WriteString("\nHow she feels about them: " + p.Feeling)
	}
	if len(p.Kept) > 0 {
		b.WriteString("\nWhat stays with her about them:")
		for _, n := range p.Kept {
			b.WriteString("\n- " + dated(n, now))
		}
	}
	notes := p.Notes
	if len(notes) > maxNotesShown {
		notes = notes[len(notes)-maxNotesShown:]
	}
	if len(notes) > 0 {
		b.WriteString("\nWhat she has noted:")
	}
	for _, n := range notes {
		b.WriteString("\n- " + dated(n, now))
	}
	if !p.LastTalked.IsZero() {
		b.WriteString("\nLast talked with her: " + ago(now.Sub(p.LastTalked)) + ".")
	}
	return b.String()
}

// dated is a note with how long ago it was.
func dated(n memory.Note, now time.Time) string {
	if n.Day.IsZero() {
		return oneLine(n.Text)
	}
	return "(" + ago(now.Sub(n.Day)) + ") " + oneLine(n.Text)
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
	line := head + ": " + clip(m.Text, maxMomentChars)
	if m.Weight >= stayedWith {
		line += " (it stayed with her)"
	}
	return line
}

// stayedWith is the weight at which a moment is shown as one that got to
// her, so the model reads it as more than a line in a log.
const stayedWith = 0.7

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
		return unfenced(strings.TrimSpace(cut[:i+1]))
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return unfenced(strings.TrimSpace(cut)) + "…"
}

// unfenced drops the backticks from a cut that left one open. Cut from a
// message with code in it, the rest of her prompt would read as code to the
// model; the words are what she remembers, not the formatting.
func unfenced(cut string) string {
	fences := strings.Count(cut, "```")
	if fences%2 == 0 && strings.Count(strings.ReplaceAll(cut, "```", ""), "`")%2 == 0 {
		return cut
	}
	return strings.ReplaceAll(cut, "`", "")
}

// oneLine flattens text into a single line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
