package core

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/config"
	"server-domme/internal/middleware"

	"server-domme/internal/version"

	"github.com/bwmarrin/discordgo"
)

type HelpUnifiedCommand struct{}

func (c *HelpUnifiedCommand) Name() string        { return "help" }
func (c *HelpUnifiedCommand) Description() string { return "Get a list of available commands" }
func (c *HelpUnifiedCommand) Group() string       { return "core" }
func (c *HelpUnifiedCommand) Category() string    { return "🕯️ Information" }
func (c *HelpUnifiedCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *HelpUnifiedCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "category",
				Description: "View commands grouped by category",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "group",
				Description: "View commands grouped by group",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "flat",
				Description: "View all commands as a flat list",
			},
		},
	}
}

func (c *HelpUnifiedCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event

	if err := bot.RespondDeferredEphemeral(session, event); err != nil {
		log.Println("[ERROR] Failed to defer help interaction:", err)
		return err
	}

	data := event.ApplicationCommandData()
	if len(data.Options) == 0 {
		return bot.FollowupEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No subcommand provided. Use `category`, `group`, or `flat`.",
		})
	}

	var output string
	switch data.Options[0].Name {
	case "group":
		output = buildHelpByGroup()
	case "flat":
		output = buildHelpFlat()
	default:
		output = buildHelpByCategory()
	}

	embed := &discordgo.MessageEmbed{
		Title:       version.AppName + " Help",
		Description: output,
		Color:       bot.EmbedColor,
	}

	return bot.FollowupEmbedEphemeral(session, event, embed)
}

func buildHelpByCategory() string {
	all := command.AllCommands()

	categoryMap := make(map[string][]command.Command)
	categorySort := make(map[string]int)

	for _, cmd := range all {
		cat := cmd.Category()
		categoryMap[cat] = append(categoryMap[cat], cmd)
		if _, ok := categorySort[cat]; !ok {
			categorySort[cat] = config.CategoryWeights[cat]
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
		sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name() < cmds[j].Name() })
		for _, cmd := range cmds {
			sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildHelpByGroup() string {
	all := command.AllCommands()

	groupMap := make(map[string][]command.Command)
	for _, cmd := range all {
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
		sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name() < cmds[j].Name() })
		for _, cmd := range cmds {
			sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildHelpFlat() string {
	all := command.AllCommands()
	sort.Slice(all, func(i, j int) bool { return all[i].Name() < all[j].Name() })

	var sb strings.Builder
	for _, cmd := range all {
		sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
	}
	return sb.String()
}

func init() {
	command.RegisterCommand(
		command.ApplyMiddlewares(
			&HelpUnifiedCommand{},
			middleware.WithGroupAccessCheck(),
			middleware.WithGuildOnly(),
			middleware.WithUserPermissionCheck(),
			middleware.WithCommandLogger(),
		),
	)
}
