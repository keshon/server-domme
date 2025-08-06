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
func (c *PurgeStopCommand) Category() string { return "🧹 Channel Cleanup" }

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

	s := slash.Session
	i := slash.Event
	storage := slash.Storage

	stopDeletion(i.ChannelID)

	_, err := storage.GetDeletionJob(i.GuildID, i.ChannelID)
	if err == nil {
		_ = storage.ClearDeletionJob(i.GuildID, i.ChannelID)
		respondEphemeral(s, i, "Message purge job stopped.")
	} else {
		respondEphemeral(s, i, "There was no purge job, but I stopped any running deletions anyway.")
	}

	logErr := logCommand(s, storage, i.GuildID, i.ChannelID, i.Member.User.ID, i.Member.User.Username, "purge-stop")
	if logErr != nil {
		log.Println("Failed to log command:", logErr)
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
