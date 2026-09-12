// Package chat is the command surface for the conversational persona: which
// channels she watches, what she is told about the server, and how the
// backends behind her are behaving.
//
// The persona itself runs in internal/chat. This package only configures it
// and feeds it messages — see ObserveMessage.
package chat

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
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
		return respond(s, e, "Pick something: `here`, `silence`, `brief`, `status` or `state`.")
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

	fmt.Fprintf(&b, "Energy %s  `%.2f`\n", meter(st.Drives.Energy), st.Drives.Energy)
	fmt.Fprintf(&b, "Alone %s  `%.2f`\n", meter(st.Drives.Social), st.Drives.Social)
	fmt.Fprintf(&b, "Interest %s  `%.2f`\n", meter(st.Drives.Interest), st.Drives.Interest)

	if st.LastSpokeAt.IsZero() {
		b.WriteString("\nShe has never spoken in this server.\n")
	} else {
		fmt.Fprintf(&b, "\nLast spoke here: <t:%d:R>\n", st.LastSpokeAt.Unix())
	}

	fmt.Fprintf(&b, "Odds of answering an indirect approach: %+.0f%%\n", st.Nudge*100)
	fmt.Fprintf(&b, "Remembers %d things here, %d bright enough to come up now\n",
		st.Memories, st.Recalled)

	// The directives verbatim, because they are the part that actually reaches
	// the model. The numbers above are how they were arrived at.
	if len(st.Irritated) > 0 {
		b.WriteString("\n**Short with**\n")
		for _, a := range st.Irritated {
			fmt.Fprintf(&b, "%s %s  `%.2f`\n", a.Username, meter(a.Level), a.Level)
		}
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

// meter draws a 0..1 value as a bar, because a column of bare decimals is
// harder to read at a glance than the shape of them.
func meter(v float64) string {
	filled := int(v*meterWidth + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > meterWidth {
		filled = meterWidth
	}
	return "`" + strings.Repeat("█", filled) + strings.Repeat("░", meterWidth-filled) + "`"
}
