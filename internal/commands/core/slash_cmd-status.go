package core

import (
	"fmt"
	"log"
	"server-domme/internal/core"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandsStatus struct{}

func (c *CommandsStatus) Name() string { return "cmd-status" }
func (c *CommandsStatus) Description() string {
	return "Check which command groups are enabled or disabled"
}
func (c *CommandsStatus) Aliases() []string  { return []string{} }
func (c *CommandsStatus) Group() string      { return "core" }
func (c *CommandsStatus) Category() string   { return "⚙️ Settings" }
func (c *CommandsStatus) RequireAdmin() bool { return false }
func (c *CommandsStatus) RequireDev() bool   { return false }

func (c *CommandsStatus) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *CommandsStatus) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session, event, storage := context.Session, context.Event, context.Storage
	guildID, member := event.GuildID, event.Member

	// Fetch disabled groups
	disabledGroups, _ := storage.GetDisabledGroups(guildID)
	disabledMap := make(map[string]bool)
	for _, g := range disabledGroups {
		disabledMap[g] = true
	}

	// Sort groups into enabled/disabled
	var enabled, disabled []string
	for _, group := range getUniqueGroups() {
		if disabledMap[group] {
			disabled = append(disabled, fmt.Sprintf("`%s`", group))
		} else {
			enabled = append(enabled, fmt.Sprintf("`%s`", group))
		}
	}

	// Prepare text for embed fields
	if len(disabled) == 0 {
		disabled = []string{"_none_"}
	}
	if len(enabled) == 0 {
		enabled = []string{"_none_"}
	}

	// Create embed message
	embed := &discordgo.MessageEmbed{
		Title: "Commands Status",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Disabled",
				Value:  strings.Join(disabled, ", "),
				Inline: false,
			},
			{
				Name:   "Enabled",
				Value:  strings.Join(enabled, ", "),
				Inline: false,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Commands are grouped (e.g., purge, core, translate). Use /help (group view) to view or /cmd-toggle to manage. Core group can’t be disabled.",
		},
	}

	// Send response
	core.RespondEmbedEphemeral(context.Session, context.Event, embed)

	// Log usage
	if err := core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name()); err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&CommandsStatus{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
		),
	)
}
