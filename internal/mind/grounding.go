package mind

import (
	"fmt"
	"strings"
	"time"
)

// Familiarity thresholds.
//
// These describe how well she knows someone, in message counts rather than in
// anything the model has to infer. They are crude on purpose: the point is
// that a stranger and a fixture of the server are addressed differently, and
// a wrong guess in the middle band costs nothing.
const (
	newcomerBelow = 5
	regularAbove  = 100
	// absenceGap is how long someone has to be gone before coming back is
	// worth noticing.
	absenceGap = 14 * 24 * time.Hour
)

// Familiarity is how well the character knows someone. Stated to the model in
// words rather than counts, because a count invites it to quote the number
// back at the person.
type Familiarity string

const (
	FamiliarityNewcomer Familiarity = "someone new here"
	FamiliarityKnown    Familiarity = "a familiar face"
	FamiliarityRegular  Familiarity = "a regular"
)

// Acquaintance is what the character knows about one person in one guild.
//
// It holds observations, not judgements: counts and timestamps the bot can
// derive from what it saw, with nothing inferred by a language model. That
// keeps this layer verifiable, and leaves room for a later pass to add the
// softer relationship state without having to untangle the two.
type Acquaintance struct {
	UserID   string
	Username string
	// Messages is how many messages the bot has seen from them in this guild.
	Messages  int
	FirstSeen time.Time
	LastSeen  time.Time
	// PrevSeen is when they were last seen before LastSeen. The gap between
	// the two is what distinguishes someone returning after a month away from
	// someone mid-conversation; LastSeen alone cannot, because it is stamped
	// with the present the moment they speak.
	PrevSeen time.Time
	// Irritation is how much they have got on her nerves, already decayed.
	Irritation float64
	// Warmth is how much she has come to like them, already decayed.
	Warmth float64
	// Facts are what they have said about themselves, newest first, and
	// Impression is her own opinion of them. See mind.Fact.
	Facts      []Fact
	Impression string
}

// Familiarity reports how well she knows this person.
func (a Acquaintance) Familiarity() Familiarity {
	switch {
	case a.Messages < newcomerBelow:
		return FamiliarityNewcomer
	case a.Messages > regularAbove:
		return FamiliarityRegular
	default:
		return FamiliarityKnown
	}
}

// AwayFor reports how long they had been gone before they last spoke, or zero
// when that gap is too short to be worth remarking on.
//
// It answers "have they just come back", not "how long since they spoke", so
// it goes quiet again once the conversation is under way: the second message
// after a return has a gap of seconds, and she should not keep greeting
// someone who is already talking to her.
func (a Acquaintance) AwayFor() time.Duration {
	if a.LastSeen.IsZero() || a.PrevSeen.IsZero() {
		return 0
	}
	gap := a.LastSeen.Sub(a.PrevSeen)
	if gap < absenceGap {
		return 0
	}
	return gap
}

// Grounding is where the character is and who she is with.
//
// This is the layer the earlier version of this package had no answer for at
// all. A persona alone produces a chatbot with a voice; behaving like a member
// of a particular community needs the community — what this server is, what
// this channel is for, and who is in the room.
type Grounding struct {
	// SelfName is the name members see on her messages in this guild — the
	// nickname if one is set, otherwise the account name.
	SelfName string
	// SelfAliases are the other things she is called: the configured name,
	// the account username, spelling variants.
	//
	// Stating these is not decoration. Discord renders a mention as the
	// account username, so a bot configured as "Dev" but named DevBot reads
	// "@DevBot test" and, told only that it is Dev, concludes DevBot is
	// somebody else and answers "wrong door". Observed in production, which is
	// why the identity line is rendered first and says plainly that all of
	// these mean her.
	SelfAliases []string

	GuildName    string
	ChannelName  string
	ChannelTopic string
	// Brief is the server's own description of itself, written by an admin.
	// It is the one piece of grounding nothing can derive, and the one that
	// most makes her sound like she belongs here rather than anywhere.
	Brief string
	// Present are the people in the current conversation.
	Present []Acquaintance
	// Now is the wall clock the grounding was built at, taken as a parameter
	// so the rendered text is reproducible in tests.
	Now time.Time
	// Drives is how she is doing: the hour, how long she has been alone, how
	// busy the room is. Not rendered here — Build turns it into instructions
	// at the end of the prompt, because rendered here as a statement of mood
	// it changed nothing. See Drives.Directives.
	Drives Drives
	// Remembers is what she still recalls of this channel, already selected
	// and ordered by Recall. Rendered at whatever detail each one has left.
	Remembers []Memory
	// Topic is the text the room is currently on, used to decide which
	// memories are close enough to the subject to surface.
	Topic string
	// AboutThem is one instruction about the person being answered, from what
	// their roles mean on this server. Built by the caller, because it needs
	// Discord's role list; see chat.Service.standing.
	AboutThem string
	// Regard is that person's combined standing, which moves their odds of an
	// answer as well as colouring it.
	Regard float64
	// Volunteering is why she is speaking when nobody asked, or empty for an
	// answer. It is the reason this message exists at all, so Build puts it
	// after everything else.
	Volunteering string
	// Flat is the instruction for answering a message that closed the topic,
	// or empty. See FlatDirective.
	Flat string
	// MayDecline lets her answer SKIP after all; see DeclineNote.
	MayDecline bool
	// Reception is how her last message to this person landed, as an
	// instruction, or empty. See ReceptionDirective.
	Reception string
	// InnerVoice asks her to write a private thought before the message; see
	// InnerVoiceNote.
	InnerVoice bool
	// Afterthought asks for a second message after her own, or is empty.
	// Build puts it after the conversation, since her own last line is what
	// it is about; see AfterthoughtDirective.
	Afterthought string
	// AnsweringAfter is how long ago the message being answered was sent,
	// set only when a reply was held back because no backend would answer.
	// Telling her the gap is what lets her acknowledge it in her own words;
	// without it she answers a stale question as though it were just asked.
	AnsweringAfter time.Duration
}

// Render turns grounding into the prompt block describing her surroundings.
// Empty fields are skipped rather than rendered as blanks, which would read to
// a model as "this is unknown" and invite it to speculate.
func (g Grounding) Render() string {
	var b strings.Builder

	if identity := g.renderIdentity(); identity != "" {
		b.WriteString(identity)
		b.WriteString("\n")
	}

	b.WriteString("Where you are right now:\n")
	if g.GuildName != "" {
		fmt.Fprintf(&b, "- The server is %s.\n", g.GuildName)
	}
	if g.Brief != "" {
		fmt.Fprintf(&b, "- What this place is: %s\n", g.Brief)
	}
	if g.ChannelName != "" {
		line := fmt.Sprintf("- You are in #%s", g.ChannelName)
		if g.ChannelTopic != "" {
			line += fmt.Sprintf(", which is for: %s", g.ChannelTopic)
		}
		b.WriteString(line + ".\n")
	}
	if clock := timeOfDay(g.Now); clock != "" {
		fmt.Fprintf(&b, "- It is %s.\n", clock)
	}

	if memories := Render(g.Remembers, g.Now, g.topicWords(), g.presentIDs()); memories != "" {
		b.WriteString("\n")
		b.WriteString(memories)
	}

	if people := g.renderPeople(); people != "" {
		b.WriteString("\nWho you are talking to:\n")
		b.WriteString(people)
	}

	// The mood is deliberately not rendered here. It was, as a fact among the
	// surroundings — "Right now: it is the dead of night and you are running
	// on fumes" — and it changed nothing measurable: at 3am the character
	// wrote the longest and liveliest reply of the set. Build emits it as
	// instructions at the very end of the prompt instead. See
	// Drives.Directives.

	return b.String()
}

// LateNote is the instruction for a reply that was held back, or "" for one
// that was not.
//
// Deliberately not part of Render. Placed among the surroundings it was
// ignored in testing — the model answered the question and said nothing about
// the delay — and moving it to the end of the prompt did not fix that either.
// What did was stamping the age on the message itself; see mind.labelled. This
// note is kept as the cheaper half of the pair, and Build puts it last.
func (g Grounding) LateNote() string {
	if g.AnsweringAfter <= 0 {
		return ""
	}
	return fmt.Sprintf(
		"You are only getting to this now, about %s after it was said, because "+
			"you were not around at the time. Open by acknowledging that in your "+
			"own words — briefly, and without apologising — then answer.",
		roughGap(g.AnsweringAfter))
}

// renderIdentity states what she is called here, and that every spelling of it
// refers to her.
func (g Grounding) renderIdentity() string {
	if g.SelfName == "" && len(g.SelfAliases) == 0 {
		return ""
	}

	var b strings.Builder
	if g.SelfName != "" {
		fmt.Fprintf(&b, "Your name here is %s. Members see that on everything you say.\n", g.SelfName)
	}
	if len(g.SelfAliases) > 0 {
		fmt.Fprintf(&b, "You are also called: %s.\n", strings.Join(g.SelfAliases, ", "))
	}
	b.WriteString("Any of those, with or without an @, means you — never someone else.\n")
	return b.String()
}

func (g Grounding) renderPeople() string {
	var b strings.Builder
	for _, p := range g.Present {
		name := p.Username
		if name == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s: %s", name, p.Familiarity())
		if away := p.AwayFor(); away > 0 {
			fmt.Fprintf(&b, ", back after about %s away", roughDuration(away))
		}
		if known := renderKnown(p.Facts, p.Impression); known != "" {
			b.WriteString(". " + known)
		}
		b.WriteString(".\n")
	}
	return b.String()
}

// timeOfDay names the part of the day, so she can sound like someone who is
// awake at this hour rather than a service with no clock. It is the caller's
// clock, which is the bot host's — good enough for a greeting, and not
// presented to the model as anything more precise than a phrase.
func timeOfDay(now time.Time) string {
	if now.IsZero() {
		return ""
	}
	switch h := now.Hour(); {
	case h < 5:
		return "the middle of the night"
	case h < 12:
		return "morning"
	case h < 18:
		return "afternoon"
	case h < 23:
		return "evening"
	default:
		return "late evening"
	}
}

// roughGap renders a short delay the way a person would say it. Separate from
// roughDuration, which deals in days and months for absences.
func roughGap(d time.Duration) string {
	switch minutes := int(d.Minutes()); {
	case minutes < 1:
		return "a moment"
	case minutes == 1:
		return "a minute"
	case minutes < 60:
		return fmt.Sprintf("%d minutes", minutes)
	default:
		return "over an hour"
	}
}

// roughDuration renders a gap the way a person would say it.
func roughDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case days >= 365:
		return "a year"
	case days >= 60:
		return fmt.Sprintf("%d months", days/30)
	case days >= 30:
		return "a month"
	default:
		return fmt.Sprintf("%d days", days)
	}
}

// topicWords is what the room is talking about, as keywords, for deciding
// which memories surface.
func (g Grounding) topicWords() []string {
	return Keywords(g.Topic)
}

// presentIDs is who is here, for the same purpose. A memory can come back
// because of who is in the room rather than what is being said.
func (g Grounding) presentIDs() []string {
	ids := make([]string, 0, len(g.Present))
	for _, p := range g.Present {
		if p.UserID != "" {
			ids = append(ids, p.UserID)
		}
	}
	return ids
}

// feelingLines are the instructions about the people present: who she is
// short with, then who she is fond of, never both for one person.
func (g Grounding) feelingLines() []string {
	var out []string
	annoyed := ""
	if who, level := g.mostIrritating(); who != "" {
		if line := IrritationDirective(who, level); line != "" {
			out = append(out, line)
			annoyed = who
		}
	}
	if who, level := g.mostLiked(annoyed); who != "" {
		if line := WarmthDirective(who, level); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// Told is every instruction about her state this grounding puts in a prompt:
// how she is, how she feels about the people present, and how her last line
// landed. The same functions Build uses, so what /chat state shows is what
// the model is told, not a second account of it.
func (g Grounding) Told() []string {
	out := append([]string(nil), g.Drives.Directives()...)
	out = append(out, g.feelingLines()...)
	if r := strings.TrimSpace(g.Reception); r != "" {
		out = append(out, r)
	}
	return out
}

// mostLiked is whoever present she is fondest of, other than skip — the
// person already getting an irritation directive, since being told to be
// short with someone and to go easy on them in one prompt is a contradiction
// the model resolves at random.
func (g Grounding) mostLiked(skip string) (string, float64) {
	var who string
	var best float64
	for _, p := range g.Present {
		if p.Username != skip && p.Warmth > best {
			who, best = p.Username, p.Warmth
		}
	}
	return who, best
}

// mostIrritating is whoever present has most got on her nerves.
//
// One name rather than a list. Two instructions about two people in one reply
// is a paragraph about her feelings, which is both more prompt than this
// deserves and the thing the character file forbids her discussing.
func (g Grounding) mostIrritating() (string, float64) {
	var who string
	var worst float64

	for _, p := range g.Present {
		if p.Irritation > worst {
			who, worst = p.Username, p.Irritation
		}
	}
	return who, worst
}
