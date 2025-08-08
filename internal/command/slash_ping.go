package command

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type PingCommand struct{}

func (c *PingCommand) Name() string        { return "ping" }
func (c *PingCommand) Description() string { return "Check bot latency" }
func (c *PingCommand) Aliases() []string   { return []string{} }

func (c *PingCommand) Group() string    { return "ping" }
func (c *PingCommand) Category() string { return "🛠️ Maintenance" }

func (c *PingCommand) RequireAdmin() bool { return false }
func (c *PingCommand) RequireDev() bool   { return false }

func (c *PingCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
	}
}

func (c *PingCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

	err := logCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

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
