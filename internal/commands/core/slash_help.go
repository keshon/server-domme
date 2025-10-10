package core

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"server-domme/internal/config"
	"server-domme/internal/core"
	"server-domme/internal/version"

	"github.com/bwmarrin/discordgo"
)

type HelpUnifiedCommand struct{}

func (c *HelpUnifiedCommand) Name() string        { return "help" }
func (c *HelpUnifiedCommand) Description() string { return "Get a list of available commands" }
func (c *HelpUnifiedCommand) Aliases() []string   { return []string{} }
func (c *HelpUnifiedCommand) Group() string       { return "core" }
func (c *HelpUnifiedCommand) Category() string    { return "🕯️ Information" }
func (c *HelpUnifiedCommand) UserPermissions() []int64 {
	return []int64{}
}

// SlashDefinition with subcommands: category, group, flat
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
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event

	if err := core.RespondDeferredEphemeral(session, event); err != nil {
		log.Println("[ERROR] Failed to defer help interaction:", err)
		return err
	}

	data := event.ApplicationCommandData()
	if len(data.Options) == 0 {
		return core.FollowupEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No subcommand provided. Use `category`, `group`, or `flat`.",
		})
	}

	var output string
	switch data.Options[0].Name {
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
		Color:       core.EmbedColor,
	}

	return core.FollowupEmbedEphemeral(session, event, embed)
}

func buildHelpByCategory(session *discordgo.Session, event *discordgo.InteractionCreate) string {
	all := core.AllCommands()

	categoryMap := make(map[string][]core.Command)
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

func buildHelpByGroup(session *discordgo.Session, event *discordgo.InteractionCreate) string {
	all := core.AllCommands()

	groupMap := make(map[string][]core.Command)
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

func buildHelpFlat(session *discordgo.Session, event *discordgo.InteractionCreate) string {
	all := core.AllCommands()
	sort.Slice(all, func(i, j int) bool { return all[i].Name() < all[j].Name() })

	var sb strings.Builder
	for _, cmd := range all {
		sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
	}
	return sb.String()
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&HelpUnifiedCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
