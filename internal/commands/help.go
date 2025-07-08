package commands

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           500,
		Name:           "help",
		Description:    "Show a list of available commands.",
		Category:       "Information",
		DCSlashHandler: helpSlashHandler,
	})
}

// Slash Discord Handler
func helpSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate

	output := buildHelpMessage()

	embed := &discordgo.MessageEmbed{
		Title:       "📖 Available Commands",
		Description: output,
		Color:       embedColor,
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err := logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "help")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}

func buildHelpMessage() string {
	cmds := All()

	categoryMap := make(map[string][]*Command)
	categorySort := make(map[string]int)
	for _, cmd := range cmds {
		cat := cmd.Category
		categoryMap[cat] = append(categoryMap[cat], cmd)

		if val, ok := categorySort[cat]; !ok || cmd.Sort < val {
			categorySort[cat] = cmd.Sort
		}
	}

	type catSortPair struct {
		Name string
		Sort int
	}
	var sortedCats []catSortPair
	for cat, sortVal := range categorySort {
		sortedCats = append(sortedCats, catSortPair{cat, sortVal})
	}
	sort.Slice(sortedCats, func(i, j int) bool {
		return sortedCats[i].Sort < sortedCats[j].Sort
	})

	var sb strings.Builder
	for _, catPair := range sortedCats {
		cat := catPair.Name
		sb.WriteString(fmt.Sprintf("**%s**\n", cat))
		cmdList := categoryMap[cat]
		sort.Slice(cmdList, func(i, j int) bool {
			return cmdList[i].Sort < cmdList[j].Sort
		})
		for _, cmd := range cmdList {
			sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name, cmd.Description))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
