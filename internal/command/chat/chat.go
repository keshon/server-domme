// Package chat is the command surface for the conversational persona: which
// channels she watches, what she is told about the server, and how the
// backends behind her are behaving.
//
// The persona itself runs in internal/chat. This package only configures it
// and feeds it messages — see ObserveMessage.
package chat

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Subcommand names. Renaming one costs every admin their muscle memory and
// forces a command re-sync, so treat them the way slash names are treated
// everywhere else here.
const (
	subHere    = "here"
	subSilence = "silence"
	subBrief   = "brief"
	subStatus  = "status"
	subState   = "state"
	subForget  = "forget"
	subRole    = "role"
	subSpeakUp = "proactive"
	subAbout   = "about"
	optUser    = "user"
	optEnabled = "enabled"
	optRole    = "role"
	optRegard  = "regard"
	optNote    = "note"
	// optConfirm is the word an administrator has to type out. A button would
	// be one click from the same mistake, and this is not undoable.
	optConfirm    = "confirm"
	confirmPhrase = "yes"
)

// ChatCommand configures the persona and feeds her every message in the
// channels she has been let into.
type ChatCommand struct {
	// Service is nil when the bot is built without a chat backend, which is
	// the ordinary state for an operator who has not opted in. Every path
	// here has to tolerate that rather than assume it away.
	Service *chatsvc.Service
	// Unavailable explains why Service is nil when the persona was switched on
	// and failed to start. Empty means it was simply never switched on.
	//
	// It is surfaced to the administrator rather than only logged: they are
	// standing in Discord holding the answer to "why isn't this working", and
	// the container logs are somewhere else entirely.
	Unavailable string
}

func (c *ChatCommand) Name() string        { return "chat" }
func (c *ChatCommand) Description() string { return "Let the resident persona into a channel" }
func (c *ChatCommand) Group() string       { return "chat" }
func (c *ChatCommand) Category() string    { return "💬 Chat" }

func (c *ChatCommand) UserPermissions() []int64 {
	// Opting a channel in sends everything said there to a third-party relay.
	// That is an administrator's call, not a member's.
	return []int64{discordgo.PermissionAdministrator}
}

// ObserveMessage implements cmdadapter.MessageObserver.
func (c *ChatCommand) ObserveMessage(ctx *cmdadapter.MessageContext) {
	if c.Service == nil {
		return
	}
	c.Service.Observe(ctx.Session, ctx.Event)
}

func (c *ChatCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subHere,
				Description: "Let her read and reply in this channel",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subSilence,
				Description: "Stop her reading this channel",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subBrief,
				Description: "Tell her what this server is, in a sentence",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "text",
						Description: "Leave empty to show what she has been told",
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subStatus,
				Description: "Where she is listening, and how the backends are holding up",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subState,
				Description: "How she is doing right now, and what that is telling her",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subRole,
				Description: "What a role means to her — how she treats anyone wearing it",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionRole,
						Name:        optRole,
						Description: "The role to set. Leave the rest empty to see what it is now",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionNumber,
						Name:        optRegard,
						Description: "-1 to 1. Negative is reserved, positive is forthcoming, 0 clears it",
						Required:    false,
						MinValue:    &minRegard,
						MaxValue:    maxRegard,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optNote,
						Description: `Said about them verbatim, e.g. "a submissive here, speak to them as one"`,
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subAbout,
				Description: "What she knows and thinks about someone",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        optUser,
						Description: "Who to look up",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subSpeakUp,
				Description: "Let her speak first here now and then — greet a regular, bring up an old thread",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionBoolean,
						Name:        optEnabled,
						Description: "On or off. Off is the default: she only ever answers",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subForget,
				Description: "Wipe everything she remembers about this server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optConfirm,
						Description: `Type "yes" — this cannot be undone`,
						Required:    true,
					},
				},
			},
		},
	}
}

func (c *ChatCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}

	s, e, store := context.Session, context.Event, context.Storage

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return respond(s, e, "Pick something: `here`, `silence`, `brief`, `status`, `state`, `about`, `role`, `proactive` or `forget`.")
	}
	sub := data.Options[0]

	if c.Service == nil {
		return respond(s, e, unavailableMessage(c.Unavailable))
	}

	switch sub.Name {
	case subHere:
		if err := store.AddChatChannel(e.GuildID, e.ChannelID); err != nil {
			return respond(s, e, "She is already listening here.")
		}
		return respond(s, e, fmt.Sprintf(
			"She can read <#%s> now, and will answer when it suits her.\n\n"+
				"Everything posted here is sent to a third-party relay to produce her "+
				"replies. Use `/chat silence` to take it back.", e.ChannelID))

	case subSilence:
		if err := store.RemoveChatChannel(e.GuildID, e.ChannelID); err != nil {
			return respond(s, e, "She was not listening here to begin with.")
		}
		// Drop what she is still holding, not just her permission to read on.
		// The conversation in memory would otherwise still be summarised, and
		// that sends it to a relay.
		c.Service.Forget(e.ChannelID)
		return respond(s, e, fmt.Sprintf("She has stopped reading <#%s>.", e.ChannelID))

	case subBrief:
		return runBrief(context, sub)

	case subStatus:
		return c.runStatus(context)

	case subState:
		return c.runState(context)

	case subRole:
		return runRole(context, sub)

	case subForget:
		return c.runForget(context, sub)

	case subSpeakUp:
		return runSpeakUp(context, sub)

	case subAbout:
		return runAbout(context, sub)

	default:
		return respond(s, e, fmt.Sprintf("Unknown subcommand: %s", sub.Name))
	}
}

func runBrief(context *cmdadapter.SlashInteractionContext, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	s, e, store := context.Session, context.Event, context.Storage

	if len(sub.Options) == 0 {
		brief := store.GetChatBrief(e.GuildID)
		if brief == "" {
			return respond(s, e,
				"She has not been told what this place is. "+
					"`/chat brief text:<a sentence>` is how she finds out.")
		}
		return respond(s, e, "She has been told:\n>>> "+brief)
	}

	brief := strings.TrimSpace(sub.Options[0].StringValue())
	if err := store.SetChatBrief(e.GuildID, brief); err != nil {
		return fmt.Errorf("chat: set brief: %w", err)
	}
	if brief == "" {
		return respond(s, e, "Cleared. She knows nothing about this place again.")
	}
	return respond(s, e, "Noted. She will keep that in mind.")
}

func (c *ChatCommand) runStatus(context *cmdadapter.SlashInteractionContext) error {
	s, e, store := context.Session, context.Event, context.Storage

	var b strings.Builder

	if name := c.Service.CharacterName(); name != "" {
		b.WriteString("**" + name + "**\n\n")
	}

	channels := store.GetChatChannels(e.GuildID)
	if len(channels) == 0 {
		b.WriteString("Listening in: nowhere yet — `/chat here` in a channel.\n")
	} else {
		b.WriteString("Listening in: ")
		for i, id := range channels {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "<#%s>", id)
		}
		b.WriteString("\n")
	}

	status := c.Service.Status()
	if status.Waiting > 0 || status.Queued > 0 {
		fmt.Fprintf(&b, "Owed replies: %d held, %d queued\n", status.Waiting, status.Queued)
	}

	if len(status.Backends) > 0 {
		b.WriteString("\n**Backends**\n")
		for _, backend := range status.Backends {
			fmt.Fprintf(&b, "`%s` — %d ok / %d failed", backend.Name, backend.Successes, backend.Failures)
			if backend.CooledFor > 0 {
				fmt.Fprintf(&b, ", resting %s", backend.CooledFor)
			}
			b.WriteString("\n")
			// Quoted only when nothing has ever worked. A backend that is
			// answering does not need its last hiccup shown to an
			// administrator; one that has never answered is the entire reason
			// they are looking at this.
			if backend.Successes == 0 && backend.LastError != "" {
				fmt.Fprintf(&b, "> %s\n", trimForEmbed(backend.LastError))
			}
		}
	}

	return respond(s, e, b.String())
}

// maxBackendErrorChars caps a quoted backend error. Discord refuses an embed
// over its own limit, and a relay can answer with an HTML error page from a
// proxy in front of it rather than one tidy JSON line.
const maxBackendErrorChars = 240

// trimForEmbed flattens an error onto one line and cuts it to something an
// embed will accept.
func trimForEmbed(msg string) string {
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > maxBackendErrorChars {
		return msg[:maxBackendErrorChars] + "…"
	}
	return msg
}

// unavailableMessage explains why there is nobody to let in.
//
// Two genuinely different situations, and telling them apart is the whole
// point: the persona was never switched on, or it was switched on and could
// not start. Reporting the first when it is the second is what sent an
// operator to check a setting they had already set.
func unavailableMessage(reason string) string {
	if reason == "" {
		return "The persona is switched off on this bot, so there is nobody to let in. " +
			"Set `CHAT_ENABLED=true` in the deployment settings and restart."
	}
	return "The persona is switched on but could not start, so there is nobody to " +
		"let in yet.\n\n" + reason
}

func respond(s *discordgo.Session, e *discordgo.InteractionCreate, msg string) error {
	return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: msg,
		Color:       reply.EmbedColor,
	})
}

// runState reports the character's current inner state.
//
// Every number here is derived at the moment it is asked for, from the same
// call the prompt uses, so what an administrator reads is what the model was
// told rather than a second copy that can drift from it.
func (c *ChatCommand) runState(context *cmdadapter.SlashInteractionContext) error {
	s, e := context.Session, context.Event
	st := c.Service.StateIn(e.GuildID, e.ChannelID)

	var b strings.Builder
	fmt.Fprintf(&b, "**How she is in <#%s>**\n\n", e.ChannelID)

	// One fenced block rather than a line each. Discord renders labels in a
	// proportional font, so "Energy" and "Interest" are different widths and
	// the bars after them do not line up; inside a code block every column
	// does.
	b.WriteString("```\n")
	fmt.Fprintf(&b, "%s\n", gauge("Energy", st.Drives.Energy))
	fmt.Fprintf(&b, "%s\n", gauge("Alone", st.Drives.Social))
	fmt.Fprintf(&b, "%s\n", gauge("Interest", st.Drives.Interest))
	b.WriteString("```\n")

	if st.LastSpokeAt.IsZero() {
		b.WriteString("\nShe has never spoken in this server.\n")
	} else {
		fmt.Fprintf(&b, "\nLast spoke here: <t:%d:R>\n", st.LastSpokeAt.Unix())
	}

	fmt.Fprintf(&b, "Odds of answering an indirect approach: %+.0f%%\n", st.Nudge*100)
	fmt.Fprintf(&b, "Remembers %d things here, %d bright enough to come up now\n",
		st.Memories, st.Recalled)
	if st.Proactive {
		fmt.Fprintf(&b, "Speaks first here: on, %d of %d used today\n",
			st.VolunteeredToday, mind.VolunteerDailyLimit)
	} else {
		b.WriteString("Speaks first here: off — she only answers\n")
	}

	// The directives verbatim, because they are the part that actually reaches
	// the model. The numbers above are how they were arrived at.
	if len(st.Irritated) > 0 {
		b.WriteString("\n**Short with**\n```\n")
		for _, a := range st.Irritated {
			fmt.Fprintf(&b, "%s\n", gauge(a.Username, a.Level))
		}
		b.WriteString("```\n")
	}

	if len(st.StyleDirective) > 0 {
		b.WriteString("\n**How she sounds** (from the character file)\n")
		for _, line := range st.StyleDirective {
			b.WriteString("- " + line + "\n")
		}
	}
	if len(st.Directives) > 0 {
		b.WriteString("\n**Right now** (computed, changes on its own)\n")
		for _, line := range st.Directives {
			b.WriteString("- " + line + "\n")
		}
	} else {
		b.WriteString("\nNothing about her mood is pronounced enough to be worth telling her.\n")
	}

	return respond(s, e, b.String())
}

// meterWidth is how many blocks a full bar draws.
const meterWidth = 10

// labelWidth is what every gauge label is padded to, so the bars all start in
// the same column.
const labelWidth = 9

// gauge draws one labelled 0..1 value, for use inside a code block.
//
// The label is padded rather than left where it falls: a column of bare
// decimals is harder to read at a glance than the shape of them, and that only
// holds if the shapes are in a column.
func gauge(label string, v float64) string {
	if len([]rune(label)) > labelWidth {
		label = string([]rune(label)[:labelWidth])
	}
	return fmt.Sprintf("%-*s %s  %.2f", labelWidth, label, meter(v), v)
}

// meter draws a 0..1 value as a bar.
func meter(v float64) string {
	filled := int(v*meterWidth + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > meterWidth {
		filled = meterWidth
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", meterWidth-filled)
}

// runForget wipes what she remembers about this server.
//
// The confirmation is a typed word rather than a button because this cannot be
// undone and there is no copy: a button is one misclick from erasing weeks of
// a character's history, and an administrator who has typed "yes" has at least
// read the sentence above it.
func (c *ChatCommand) runForget(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e, store := context.Session, context.Event, context.Storage

	var confirm string
	for _, opt := range sub.Options {
		if opt.Name == optConfirm {
			confirm = strings.TrimSpace(strings.ToLower(opt.StringValue()))
		}
	}
	if confirm != confirmPhrase {
		return respond(s, e, fmt.Sprintf(
			"Nothing was touched. To wipe what she remembers about this server, "+
				"run `/chat forget confirm:%s` — it cannot be undone.", confirmPhrase))
	}

	forgotten, err := store.ForgetMindMemories(e.GuildID)
	if err != nil {
		return fmt.Errorf("chat: forget memories: %w", err)
	}

	return respond(s, e, fmt.Sprintf(
		"Forgotten: %d things she remembered about this server, what she knew "+
			"and thought about the people in it, and how she felt about them.\n\n"+
			"She still knows who is a regular and who is new — that is counted from "+
			"messages, not remembered, and wiping it would turn everyone here into a "+
			"stranger.", forgotten))
}

// runAbout shows her file on one person: how well she knows them, how she
// feels about them, her opinion and what they have told her.
//
// Shown to administrators because it is kept about members without asking
// them, and the people running a server should be able to see exactly what
// that amounts to rather than take it on trust.
func runAbout(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e, store := context.Session, context.Event, context.Storage

	var userID string
	for _, opt := range sub.Options {
		if opt.Name == optUser {
			userID, _ = opt.Value.(string)
		}
	}

	p := store.GetMindPerson(e.GuildID, userID)
	if p == nil {
		return respond(s, e, fmt.Sprintf("She has never seen <@%s> say anything.", userID))
	}

	now := time.Now()
	who := mind.Acquaintance{Messages: p.Messages}

	var b strings.Builder
	fmt.Fprintf(&b, "**<@%s>** — %s, %d messages seen, first <t:%d:R>\n",
		userID, who.Familiarity(), p.Messages, p.FirstSeen.Unix())

	b.WriteString("```\n")
	fmt.Fprintf(&b, "%s\n", gauge("Warmth", mind.WarmthNow(p.Warmth, p.WarmAt, now)))
	fmt.Fprintf(&b, "%s\n", gauge("Irritated", mind.IrritationNow(p.Irritation, p.IrritatedAt, now)))
	b.WriteString("```\n")

	if p.Impression != "" {
		fmt.Fprintf(&b, "**Her take** (<t:%d:R>)\n> %s\n", p.ImpressionAt.Unix(), p.Impression)
	}
	if len(p.Facts) > 0 {
		b.WriteString("\n**What they have told her**\n")
		for _, f := range p.Facts {
			fmt.Fprintf(&b, "- %s: %s (<t:%d:R>)\n", strings.ReplaceAll(f.Key, "_", " "), f.Value, f.At.Unix())
		}
	}
	if p.Impression == "" && len(p.Facts) == 0 {
		b.WriteString("\nShe has no opinion of them yet and nothing they have told her. " +
			"Both are written after a conversation she remembers.\n")
	}

	return respond(s, e, b.String())
}

// runSpeakUp switches volunteering on or off for this channel.
//
// Per channel rather than per server, because what is welcome differs: a
// general channel can take her greeting someone back, a support channel
// cannot take her bringing up last week's argument.
func runSpeakUp(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e, store := context.Session, context.Event, context.Storage

	var on bool
	for _, opt := range sub.Options {
		if opt.Name == optEnabled {
			on = opt.BoolValue()
		}
	}

	if err := store.SetChatProactive(e.GuildID, e.ChannelID, on); err != nil {
		if errors.Is(err, storage.ErrChatChannelRequired) {
			return respond(s, e, "She is not listening here yet. `/chat here` first.")
		}
		return fmt.Errorf("chat: set proactive: %w", err)
	}

	if !on {
		return respond(s, e, fmt.Sprintf("She will only answer in <#%s> now.", e.ChannelID))
	}
	return respond(s, e, fmt.Sprintf(
		"She may speak first in <#%s> now: noticing a regular who has been gone "+
			"a while, or bringing up something she remembers when it comes round "+
			"again.\n\n"+
			"At most %d times a day, never within %d minutes of the last, never "+
			"while she is already in the conversation and never when she is worn "+
			"out. `/chat proactive enabled:false` stops it.",
		e.ChannelID, mind.VolunteerDailyLimit, int(mind.VolunteerCooldown.Minutes())))
}

// Regard bounds, as Discord enforces them in the picker so a bad value never
// reaches the bot.
var (
	minRegard = -1.0
	maxRegard = 1.0
)

// runRole sets or shows what a Discord role means to the persona.
//
// Per role rather than per person because a server that has roles has already
// decided who is what. Asking an operator to rate three hundred members one at
// a time is asking them not to use it.
func runRole(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e, store := context.Session, context.Event, context.Storage

	var roleID, note string
	var regard float64
	var setting bool

	for _, opt := range sub.Options {
		switch opt.Name {
		case optRole:
			roleID = opt.Value.(string)
		case optRegard:
			regard = opt.FloatValue()
			setting = true
		case optNote:
			note = strings.TrimSpace(opt.StringValue())
			setting = true
		}
	}
	if roleID == "" {
		return respond(s, e, "Name a role.")
	}

	if !setting {
		bias, ok := store.ChatRoleBiases(e.GuildID)[roleID]
		if !ok {
			return respond(s, e, fmt.Sprintf(
				"<@&%s> means nothing to her in particular. "+
					"`/chat role role:<role> regard:<-1..1> note:<what they are>` changes that.",
				roleID))
		}
		return respond(s, e, describeBias(roleID, bias))
	}

	bias := storage.ChatRoleBias{Regard: regard, Note: note}
	if err := store.SetChatRoleBias(e.GuildID, roleID, bias); err != nil {
		return fmt.Errorf("chat: set role bias: %w", err)
	}

	if bias.Regard == 0 && bias.Note == "" {
		return respond(s, e, fmt.Sprintf("<@&%s> means nothing to her again.", roleID))
	}
	return respond(s, e, describeBias(roleID, bias))
}

// describeBias says what a role is worth, and what it will actually put in
// front of her — the directive rather than the number, because the number is
// not what she reads.
func describeBias(roleID string, bias storage.ChatRoleBias) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<@&%s>\n```\n%s\n```\n", roleID, gauge("regard", bias.Regard))

	if line := mind.RegardDirective("Someone", bias.Note, bias.Regard); line != "" {
		b.WriteString("She is told: " + line)
	} else {
		b.WriteString("Not enough either way to be worth telling her.")
	}
	return b.String()
}
