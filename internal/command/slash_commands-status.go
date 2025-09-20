package command

import (
	"fmt"
	"log"
	"server-domme/internal/core"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandsStatus struct{}

func (c *CommandsStatus) Name() string        { return "commands-status" }
func (c *CommandsStatus) Description() string { return "Check which command is enabled or disabled" }
func (c *CommandsStatus) Aliases() []string   { return []string{} }
func (c *CommandsStatus) Group() string       { return "core" }
func (c *CommandsStatus) Category() string    { return "⚙️ Settings" }
func (c *CommandsStatus) RequireAdmin() bool  { return true }
func (c *CommandsStatus) RequireDev() bool    { return false }

func (c *CommandsStatus) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options:     []*discordgo.ApplicationCommandOption{},
	}
}

func (c *CommandsStatus) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID
	member := event.Member

	disabledGroups, _ := context.Storage.GetDisabledGroups(guildID)
	disabledMap := make(map[string]bool)
	for _, g := range disabledGroups {
		disabledMap[g] = true
	}

	var enabled []string
	var disabled []string

	for _, group := range getUniqueGroups() {
		if disabledMap[group] {
			disabled = append(disabled, fmt.Sprintf("`%s`", group))
		} else {
			enabled = append(enabled, fmt.Sprintf("`%s`", group))
		}
	}

	var sb strings.Builder

	sb.WriteString("**Disabled**\n")
	if len(disabled) > 0 {
		sb.WriteString(strings.Join(disabled, ", "))
	} else {
		sb.WriteString("_none_")
	}

	sb.WriteString("\n\n**Enabled**\n")
	if len(enabled) > 0 {
		sb.WriteString(strings.Join(enabled, ", "))
	} else {
		sb.WriteString("_none_")
	}

	err := core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return core.RespondEphemeral(context.Session, context.Event, sb.String())
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&CommandsStatus{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
