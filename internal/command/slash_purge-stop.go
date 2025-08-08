package command

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type PurgeStopCommand struct{}

func (c *PurgeStopCommand) Name() string        { return "purge-stop" }
func (c *PurgeStopCommand) Description() string { return "Halt ongoing purge in this channel" }
func (c *PurgeStopCommand) Aliases() []string   { return []string{} }

func (c *PurgeStopCommand) Group() string    { return "purge" }
func (c *PurgeStopCommand) Category() string { return "🧹 Cleanup" }

func (c *PurgeStopCommand) RequireAdmin() bool { return true }
func (c *PurgeStopCommand) RequireDev() bool   { return false }

func (c *PurgeStopCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options:     []*discordgo.ApplicationCommandOption{},
	}
}

func (c *PurgeStopCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

	stopDeletion(event.ChannelID)

	_, err := storage.GetDeletionJob(event.GuildID, event.ChannelID)
	if err == nil {
		_ = storage.ClearDeletionJob(event.GuildID, event.ChannelID)
		respondEphemeral(session, event, "Message purge job stopped.")
	} else {
		respondEphemeral(session, event, "There was no purge job, but I stopped any running deletions anyway.")
	}

	err = logCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&PurgeStopCommand{},
			),
		),
	)
}
