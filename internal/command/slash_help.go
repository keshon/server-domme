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
				Name:        "view_as",
				Description: "View commands as categories, groups, or a flat list",
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

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

	viewAs := "category"
	opts := event.ApplicationCommandData().Options
	if len(opts) > 0 {
		viewAs = opts[0].StringValue()
	}

	var output string
	switch viewAs {
	case "group":
		output = buildHelpByGroup(session, event)
	case "flat":
		output = buildHelpFlat(session, event)
	default:
		output = buildHelpByCategory(session, event)
	}

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

	err = logCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
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

func buildHelpByCategory(session *discordgo.Session, event *discordgo.InteractionCreate) string {
	userID := event.Member.User.ID
	all := All()

	categoryMap := make(map[string][]Command)
	categorySort := make(map[string]int)

	for _, cmd := range all {
		if cmd.RequireAdmin() && !isAdministrator(session, event.GuildID, event.Member) {
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

func buildHelpByGroup(session *discordgo.Session, event *discordgo.InteractionCreate) string {
	userID := event.Member.User.ID
	all := All()

	groupMap := make(map[string][]Command)

	for _, cmd := range all {
		if cmd.RequireAdmin() && !isAdministrator(session, event.GuildID, event.Member) {
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

func buildHelpFlat(session *discordgo.Session, event *discordgo.InteractionCreate) string {
	userID := event.Member.User.ID
	all := All()

	var cmds []Command
	for _, cmd := range all {
		if cmd.RequireAdmin() && !isAdministrator(session, event.GuildID, event.Member) {
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
