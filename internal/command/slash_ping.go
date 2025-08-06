package command

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

type PingCommand struct{}

func (p *PingCommand) Name() string        { return "ping" }
func (p *PingCommand) Description() string { return "Check bot latency" }
func (p *PingCommand) Aliases() []string   { return []string{} }

func (c *PingCommand) Group() string    { return "ping" }
func (p *PingCommand) Category() string { return "🧪 Test" }

func (p *PingCommand) RequireAdmin() bool { return false }
func (p *PingCommand) RequireDev() bool   { return false }

func (p *PingCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        p.Name(),
		Description: p.Description(),
		Type:        discordgo.ChatApplicationCommand,
	}
}

func (p *PingCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event

	latency := session.HeartbeatLatency().Milliseconds()
	return session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🏓 Pong! %dms", latency),
		},
	})
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&PingCommand{},
			),
		),
	)
}
