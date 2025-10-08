package purge

import (
	"log"
	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type PurgeStopCommand struct{}

func (c *PurgeStopCommand) Name() string        { return "purge-stop" }
func (c *PurgeStopCommand) Description() string { return "Halt ongoing purge in this channel" }
func (c *PurgeStopCommand) Aliases() []string   { return []string{} }
func (c *PurgeStopCommand) Group() string       { return "purge" }
func (c *PurgeStopCommand) Category() string    { return "🧹 Cleanup" }
func (c *PurgeStopCommand) RequireAdmin() bool  { return true }
func (c *PurgeStopCommand) RequireDev() bool    { return false }

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

	guildID := event.GuildID
	member := event.Member

	stopDeletion(event.ChannelID)

	_, err := storage.GetDeletionJob(event.GuildID, event.ChannelID)
	if err == nil {
		_ = storage.ClearDeletionJob(event.GuildID, event.ChannelID)
		core.RespondEphemeral(session, event, "Message purge job stopped.")
	} else {
		core.RespondEphemeral(session, event, "There was no purge job, but I stopped any running deletions anyway.")
	}

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&PurgeStopCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
		),
	)
}
