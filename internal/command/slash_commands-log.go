package command

import (
	"fmt"
	"log"
	"server-domme/internal/core"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	discordMaxMessageLength = 2000
	codeLeftBlockWrapper    = "```md"
	codeRightBlockWrapper   = "```"
)

var maxContentLength = discordMaxMessageLength - len(codeLeftBlockWrapper) - len(codeRightBlockWrapper)

type LogCommand struct{}

func (c *LogCommand) Name() string        { return "commands-log" }
func (c *LogCommand) Description() string { return "Review recent commands and their punishments" }
func (c *LogCommand) Aliases() []string   { return []string{} }
func (c *LogCommand) Group() string       { return "core" }
func (c *LogCommand) Category() string    { return "⚙️ Settings" }
func (c *LogCommand) RequireAdmin() bool  { return true }
func (c *LogCommand) RequireDev() bool    { return false }

func (c *LogCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *LogCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

	if !core.IsAdministrator(session, guildID, member) {
		core.RespondEphemeral(session, event, "You must be an Admin to use this command, darling.")
		return nil
	}

	records, err := storage.GetCommands(guildID)
	if err != nil {
		core.RespondEphemeral(session, event, fmt.Sprintf("Failed to fetch command logs: %v", err))
		return nil
	}

	if len(records) == 0 {
		core.RespondEphemeral(session, event, "No command history found. Such a quiet guild, or lazy users.")
		return nil
	}

	var builder strings.Builder
	header := fmt.Sprintf("%-19s\t%-15s\t%-12s\t%s\n", "# Datetime", "# Username", "# Channel", "# Command")
	builder.WriteString(header)

	for idx := len(records) - 1; idx >= 0; idx-- {
		r := records[idx]

		username := r.Username
		if r.Command == "confess" && !core.IsDeveloper(member.User.ID) {
			username = "###"
		}

		line := fmt.Sprintf(
			"%-19s\t%-15s\t#%-12s\t/%s\n",
			r.Datetime.Format("2006-01-02 15:04:05"),
			username,
			r.ChannelName,
			r.Command,
		)

		if builder.Len()+len(line) > maxContentLength {
			break
		}

		builder.WriteString(line)
	}

	out := codeLeftBlockWrapper + "\n" + builder.String() + codeRightBlockWrapper
	core.RespondEphemeral(session, event, out)

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&LogCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
