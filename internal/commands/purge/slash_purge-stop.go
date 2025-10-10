package purge

import (
	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type PurgeStopCommand struct{}

func (c *PurgeStopCommand) Name() string        { return "purge-stop" }
func (c *PurgeStopCommand) Description() string { return "Halt ongoing purge in this channel" }
func (c *PurgeStopCommand) Aliases() []string   { return []string{} }
func (c *PurgeStopCommand) Group() string       { return "purge" }
func (c *PurgeStopCommand) Category() string    { return "🧹 Cleanup" }
func (c *PurgeStopCommand) UserPermissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}
func (c *PurgeStopCommand) BotPermissions() []int64 {
	return []int64{}
}

func (c *PurgeStopCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options:     []*discordgo.ApplicationCommandOption{},
	}
}

func (c *PurgeStopCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	stopDeletion(event.ChannelID)

	_, err := storage.GetDeletionJob(event.GuildID, event.ChannelID)
	if err == nil {
		_ = storage.ClearDeletionJob(event.GuildID, event.ChannelID)
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{Description: "Message purge job stopped."})
	} else {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{Description: "There is no active purge job in this channel."})
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&PurgeStopCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithBotPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
