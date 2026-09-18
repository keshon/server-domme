package chat

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/mind"
)

const optLevel = "level"

// AttentionCommand lets a member say how much the persona may come after
// them. A member's command, not an administrator's: it is consent, and only
// the person giving it can.
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
				Type: discordgo.ApplicationCommandOptionString, Name: optLevel,
				Description: "How much you will put up with. Empty shows what you have now",
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "off — she never comes after you", Value: "off"},
					{Name: "light — at most once a day", Value: string(mind.AttentionLight)},
					{Name: "keen — up to three times a day", Value: string(mind.AttentionKeen)},
					{Name: "insistent — up to six times a day", Value: string(mind.AttentionInsistent)},
				},
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
	userID := e.Member.User.ID

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		current := "off"
		if p := store.GetMindPerson(e.GuildID, userID); p != nil && p.Attention != "" {
			current = p.Attention
		}
		return respond(s, e, fmt.Sprintf("You have it on **%s**. `/attention level:` changes it.", current))
	}

	level, ok := mind.ParseAttention(data.Options[0].StringValue())
	if !ok {
		return respond(s, e, "That is not a level.")
	}
	if err := store.SetMindAttention(e.GuildID, userID, string(level), time.Now()); err != nil {
		return fmt.Errorf("chat: set attention: %w", err)
	}

	if level == mind.AttentionOff {
		return respond(s, e, "Off. She will not come after you. She still answers when you talk to her.")
	}
	msg := fmt.Sprintf("**%s**. She may come after you when she wants your attention — "+
		"when she has missed you, or you are around and ignoring her. Whether she does "+
		"is up to how she feels about you; this is only how much you will put up with.\n\n"+
		"She backs off if you do not answer, never at night, and stops at once if you tell "+
		"her to leave you alone. `/attention level:off` ends it.", level)
	if store.IsChatAttentionOff(e.GuildID) {
		msg += "\n\n⚠️ An administrator has switched this off for the whole server, so for now nothing will happen."
	}
	return respond(s, e, msg)
}
