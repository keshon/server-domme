package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Name:           "help",
		Description:    "Show a list of available commands.",
		Category:       "Utility",
		DCSlashHandler: helpSlashHandler,
	})
}

// Slash Discord Handler
func helpSlashHandler(ctx *SlashContext) {
	output := buildHelpMessage()

	embed := &discordgo.MessageEmbed{
		Title:       "📖 Available Commands",
		Description: output,
		Color:       embedColor,
	}
	ctx.Session.InteractionRespond(ctx.Interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// Helper to build the command list output
func buildHelpMessage() string {
	cmds := All()

	// Group commands by category
	categoryMap := make(map[string][]*Command)
	for _, cmd := range cmds {
		cat := cmd.Category
		categoryMap[cat] = append(categoryMap[cat], cmd)
	}

	var categories []string
	for cat := range categoryMap {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	var sb strings.Builder
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("**%s**\n", cat))
		cmdList := categoryMap[cat]
		sort.Slice(cmdList, func(i, j int) bool {
			return cmdList[i].Name < cmdList[j].Name
		})
		for _, cmd := range cmdList {
			sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name, cmd.Description))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
