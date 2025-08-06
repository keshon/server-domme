package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandsStatus struct{}

func (c *CommandsStatus) Name() string { return "command-status" }
func (c *CommandsStatus) Description() string {
	return "Check which command groups are enabled or disabled"
}
func (c *CommandsStatus) Aliases() []string { return []string{} }

func (c *CommandsStatus) Group() string    { return "core" }
func (c *CommandsStatus) Category() string { return "⚙️ Settings" }

func (c *CommandsStatus) RequireAdmin() bool { return true }
func (c *CommandsStatus) RequireDev() bool   { return false }

func (c *CommandsStatus) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options:     []*discordgo.ApplicationCommandOption{},
	}
}

func (c *CommandsStatus) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("invalid context")
	}

	guildID := slash.Event.GuildID
	disabledGroups, _ := slash.Storage.GetDisabledGroups(guildID)
	disabledMap := make(map[string]bool)
	for _, g := range disabledGroups {
		disabledMap[g] = true
	}

	var sb strings.Builder
	sb.WriteString("Command groups status:\n\n")

	groups := getUniqueGroups()
	for _, group := range groups {
		status := "✅ enabled"
		if disabledMap[group] {
			status = "🚫 disabled"
		}
		sb.WriteString(fmt.Sprintf("• `%s`: %s\n", group, status))
	}

	return respondEphemeral(slash.Session, slash.Event, sb.String())
}

func init() {
	Register(WithGuildOnly(WithGroupAccessCheck()(&CommandsStatus{})))
}

func getUniqueGroups() []string {
	set := map[string]struct{}{}
	for _, cmd := range All() {
		group := cmd.Group()
		if group != "" {
			set[group] = struct{}{}
		}
	}
	var result []string
	for group := range set {
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}
