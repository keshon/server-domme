package chat

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/perm"
)

const optWholeServer = "whole_server"

// AttentionCommand is how a member says the persona may come after them, and
// how an administrator stops her doing it to anyone.
//
// One command for both. Consent is a member's to give and only they can give
// it; the server-wide switch sits in the same command behind an administrator
// check, rather than in a second command with a confusingly similar name.
type AttentionCommand struct {
	// Service is nil when the persona is not running.
	Service *chatsvc.Service
}

func (c *AttentionCommand) Name() string { return "attention" }
func (c *AttentionCommand) Description() string {
	return "Let her come after you when she wants your attention — or stop her"
}
func (c *AttentionCommand) Group() string            { return "chat" }
func (c *AttentionCommand) Category() string         { return "💬 Chat" }
func (c *AttentionCommand) UserPermissions() []int64 { return []int64{} }

func (c *AttentionCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type: discordgo.ApplicationCommandOptionBoolean, Name: optEnabled,
				Description: "On lets her come after you; off stops her. Empty shows what you have",
			},
			{
				Type: discordgo.ApplicationCommandOptionBoolean, Name: optWholeServer,
				Description: "Administrators: apply it to the whole server instead of just you",
			},
		},
	}
}

func (c *AttentionCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}
	s, e, store := context.Session, context.Event, context.Storage
	if c.Service == nil {
		return respond(s, e, "The persona is not running on this bot, so there is nobody to come after you.")
	}

	var enabled, wholeServer *bool
	for _, o := range e.ApplicationCommandData().Options {
		v := o.BoolValue()
		switch o.Name {
		case optEnabled:
			enabled = &v
		case optWholeServer:
			wholeServer = &v
		}
	}

	if wholeServer != nil && *wholeServer {
		if !perm.IsAdministrator(s, e.Member, context.Config) {
			return respond(s, e, "Only administrators can change this for the whole server. Leave `whole_server` off to change it for yourself.")
		}
		if enabled == nil {
			return respond(s, e, serverStatus(store.IsChatAttentionOff(e.GuildID)))
		}
		if err := store.SetChatAttentionOff(e.GuildID, !*enabled); err != nil {
			return fmt.Errorf("chat: set attention: %w", err)
		}
		return respond(s, e, serverStatus(!*enabled))
	}

	userID := e.Member.User.ID
	if enabled == nil {
		on := false
		if p := store.GetMindPerson(e.GuildID, userID); p != nil {
			on = chatsvc.Consented(p.Attention)
		}
		msg := "Off: she never comes after you."
		if on {
			msg = "On: she may come after you when she wants your attention."
		}
		if store.IsChatAttentionOff(e.GuildID) {
			msg += "\n\nAn administrator has switched this off for the whole server, so for now nothing happens either way."
		}
		return respond(s, e, msg)
	}

	value := ""
	if *enabled {
		value = chatsvc.ConsentOn
	}
	if err := store.SetMindConsent(e.GuildID, userID, value, time.Now()); err != nil {
		return fmt.Errorf("chat: set consent: %w", err)
	}
	if !*enabled {
		return respond(s, e, "Off. She will not come after you. She still answers when you talk to her.")
	}

	msg := "On. She may come after you when she wants your attention — when she has missed you, " +
		"or you are around and ignoring her.\n\n" +
		"Whether she does is up to how she feels about you, and how often is up to how you take it: " +
		"answer her and she comes back sooner, ignore her and she backs off, and after a few " +
		"unanswered she stops until you speak to her. Never at night. Tell her to leave you alone, " +
		"or run `/attention enabled:false`, and it ends at once."
	if store.IsChatAttentionOff(e.GuildID) {
		msg += "\n\n⚠️ An administrator has switched this off for the whole server, so for now nothing will happen."
	}
	return respond(s, e, msg)
}

func serverStatus(off bool) string {
	if off {
		return "Off for the whole server: she will not come after anyone here, whatever they agreed to. She still answers."
	}
	return "On for the server: she may come after members who turned it on for themselves."
}
