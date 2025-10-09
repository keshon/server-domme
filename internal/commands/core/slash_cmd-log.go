package core

import (
	"fmt"
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

func (c *LogCommand) Name() string        { return "cmd-log" }
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
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID
	member := event.Member

	// Fetch command logs
	records, err := storage.GetCommands(guildID)
	if err != nil {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to fetch command logs: %v", err),
		})
		return nil
	}
	if len(records) == 0 {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No command logs found.",
		})
		return nil
	}

	// Build table
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%-19s\t%-15s\t%-12s\t%s\n", "# Datetime", "# Username", "# Channel", "# Command"))

	// Add logs in reverse order (latest first)
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]

		// Hide usernames for 'confess' if not a developer
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

		// Stop if message too long
		if builder.Len()+len(line) > maxContentLength {
			break
		}

		builder.WriteString(line)
	}

	// Wrap in code block
	msg := codeLeftBlockWrapper + "\n" + builder.String() + codeRightBlockWrapper
	core.RespondEphemeral(session, event, msg)

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&LogCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithCommandLogger(),
		),
	)
}
