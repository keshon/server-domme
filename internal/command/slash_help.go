package command

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"server-domme/internal/version"

	"github.com/bwmarrin/discordgo"
)

type HelpUnifiedCommand struct{}

func (c *HelpUnifiedCommand) Name() string        { return "help" }
func (c *HelpUnifiedCommand) Description() string { return "Get a list of available commands" }
func (c *HelpUnifiedCommand) Aliases() []string   { return []string{} }

func (c *HelpUnifiedCommand) Group() string    { return "core" }
func (c *HelpUnifiedCommand) Category() string { return "🕯️ Information" }

func (c *HelpUnifiedCommand) RequireAdmin() bool { return false }
func (c *HelpUnifiedCommand) RequireDev() bool   { return false }

func (c *HelpUnifiedCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "view_by",
				Description: "How to view the commands",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Categories", Value: "category"},
					{Name: "Groups", Value: "group"},
					{Name: "Flat list", Value: "flat"},
				},
			},
		},
	}
}

func (c *HelpUnifiedCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	s := slash.Session
	e := slash.Event
	st := slash.Storage

	viewBy := "category"
	opts := e.ApplicationCommandData().Options
	if len(opts) > 0 {
		viewBy = opts[0].StringValue()
	}

	var output string
	switch viewBy {
	case "group":
		output = buildHelpByGroup(s, e)
	case "flat":
		output = buildHelpFlat(s, e)
	default:
		output = buildHelpByCategory(s, e)
	}

	embed := &discordgo.MessageEmbed{
		Title:       version.AppName + " Help",
		Description: output,
		Color:       embedColor,
	}

	err := s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
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

	logErr := logCommand(s, st, e.GuildID, e.ChannelID, e.Member.User.ID, e.Member.User.Username, "help ("+viewBy+")")
	if logErr != nil {
		log.Println("Failed to log help command:", logErr)
	}
	return nil
}

var categoryWeights = map[string]int{
	"🕯️ Information": 0,
	"📢 Utilities":    10,
	"🎲 Gameplay":     20,
	"🎭 Roleplay":     30,
	"🧹 Cleanup":      40,
	"⚙️ Settings":    50,
	"🛠️ Maintenance": 60,
}

func buildHelpByCategory(s *discordgo.Session, i *discordgo.InteractionCreate) string {
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
		if _, ok := categorySort[cat]; !ok {
			categorySort[cat] = categoryWeights[cat]
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
			return cmds[i].Name() < cmds[j].Name()
		})
		for _, cmd := range cmds {
			sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildHelpByGroup(s *discordgo.Session, i *discordgo.InteractionCreate) string {
	userID := i.Member.User.ID
	all := All()

	groupMap := make(map[string][]Command)

	for _, cmd := range all {
		if cmd.RequireAdmin() && !isAdministrator(s, i.GuildID, i.Member) {
			continue
		}
		if cmd.RequireDev() && !isDeveloper(userID) {
			continue
		}
		group := cmd.Group()
		groupMap[group] = append(groupMap[group], cmd)
	}

	var sortedGroups []string
	for group := range groupMap {
		sortedGroups = append(sortedGroups, group)
	}
	sort.Strings(sortedGroups)

	var sb strings.Builder
	for _, group := range sortedGroups {
		sb.WriteString(fmt.Sprintf("**%s**\n", group))
		cmds := groupMap[group]
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].Name() < cmds[j].Name()
		})
		for _, cmd := range cmds {
			sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildHelpFlat(s *discordgo.Session, i *discordgo.InteractionCreate) string {
	userID := i.Member.User.ID
	all := All()

	var cmds []Command
	for _, cmd := range all {
		if cmd.RequireAdmin() && !isAdministrator(s, i.GuildID, i.Member) {
			continue
		}
		if cmd.RequireDev() && !isDeveloper(userID) {
			continue
		}
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name() < cmds[j].Name()
	})

	var sb strings.Builder
	for _, cmd := range cmds {
		sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
	}
	return sb.String()
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&HelpUnifiedCommand{},
			),
		),
	)
}
