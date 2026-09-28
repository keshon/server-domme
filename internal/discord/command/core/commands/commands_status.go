package commands

import (
	"fmt"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// RunStatus reports enabled and disabled command groups.
func RunStatus(ctx *adapter.SlashInteractionContext) error {
	disabledGroups, _ := ctx.Storage.DisabledGroups(ctx.GuildID())
	disabledMap := make(map[string]bool)
	for _, g := range disabledGroups {
		disabledMap[g] = true
	}

	var enabled, disabled []string
	for _, group := range getUniqueGroups() {
		if disabledMap[group] {
			disabled = append(disabled, fmt.Sprintf("`%s`", group))
		} else {
			enabled = append(enabled, fmt.Sprintf("`%s`", group))
		}
	}

	if len(disabled) == 0 {
		disabled = []string{"_none_"}
	}
	if len(enabled) == 0 {
		enabled = []string{"_none_"}
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Title:       "Commands Status",
		Description: "Commands are grouped (e.g., purge, core, translate). Use `/help category` to view or `/settings commands enable` / `/settings commands disable` to manage. Core group can't be disabled.",
		Fields: []adapter.EmbedField{
			{Name: "Disabled", Value: strings.Join(disabled, ", "), Inline: false},
			{Name: "Enabled", Value: strings.Join(enabled, ", "), Inline: false},
		},
	})
}
