package command

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"server-domme/internal/version"

	"github.com/bwmarrin/discordgo"
)

type HelpCommand struct{}

func (c *HelpCommand) Name() string        { return "help" }
func (c *HelpCommand) Description() string { return "Your guide to serving the Server Domme well" }
func (c *HelpCommand) Aliases() []string   { return []string{} }

func (c *HelpCommand) Group() string    { return "help" }
func (c *HelpCommand) Category() string { return "🕯️ Insight" }

func (c *HelpCommand) RequireAdmin() bool { return false }
func (c *HelpCommand) RequireDev() bool   { return false }

func (c *HelpCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *HelpCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}
	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	output := buildHelpMessage(session, event)

	embed := &discordgo.MessageEmbed{
		Title:       version.AppName + " Help",
		Description: output,
		Color:       embedColor,
	}

	err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println("Failed to send help embed:", err)
		return nil
	}

	logErr := logCommand(session, storage, event.GuildID, event.ChannelID, event.Member.User.ID, event.Member.User.Username, "help")
	if logErr != nil {
		log.Println("Failed to log help command:", logErr)
	}
	return nil
}

func buildHelpMessage(s *discordgo.Session, i *discordgo.InteractionCreate) string {
	userID := i.Member.User.ID
	all := All()

	categoryMap := make(map[string][]Command)
	categorySort := make(map[string]int)

	for _, cmd := range all {
		if cmd.RequireAdmin() && !isAdministrator(s, i.GuildID, i.Member) {
			continue
		}
		if cmd.RequireDev() && !isDeveloper(userID) {
			continue
		}
		cat := cmd.Category()
		categoryMap[cat] = append(categoryMap[cat], cmd)

		if val, ok := categorySort[cat]; !ok || cmdOrder(cmd) < val {
			categorySort[cat] = cmdOrder(cmd)
		}
	}

	type catSort struct {
		Name string
		Sort int
	}
	var sortedCats []catSort
	for cat, sortVal := range categorySort {
		sortedCats = append(sortedCats, catSort{cat, sortVal})
	}
	sort.Slice(sortedCats, func(i, j int) bool {
		return sortedCats[i].Sort < sortedCats[j].Sort
	})

	var sb strings.Builder
	for _, cat := range sortedCats {
		sb.WriteString(fmt.Sprintf("**%s**\n", cat.Name))
		cmds := categoryMap[cat.Name]
		sort.Slice(cmds, func(i, j int) bool {
			return cmdOrder(cmds[i]) < cmdOrder(cmds[j])
		})
		for _, cmd := range cmds {
			sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func init() {
	Register(WithGuildOnly(&HelpCommand{}))
}

// optional: define command sort order fallback if needed
func cmdOrder(cmd Command) int {
	if sd, ok := cmd.(interface{ Sort() int }); ok {
		return sd.Sort()
	}
	return 999
}
