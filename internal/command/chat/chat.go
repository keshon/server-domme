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
		return respond(s, e, "Pick something: `here`, `silence`, `brief` or `status`.")
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
		return respond(s, e, fmt.Sprintf("She has stopped reading <#%s>.", e.ChannelID))

	case subBrief:
		return runBrief(context, sub)

	case subStatus:
		return c.runStatus(context)

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
		}
	}

	return respond(s, e, b.String())
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
