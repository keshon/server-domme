package discipline

import (
	"fmt"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// ManageRolesOptions returns the settings options for discipline role
// management. Wired as the discipline group under /settings.
func ManageRolesOptions() []adapter.SlashOption {
	return []adapter.SlashOption{
		{
			Type:        adapter.OptionSubCommand,
			Name:        "roles-set",
			Description: "Configure discipline roles",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionString,
					Name:        "type",
					Description: "Which role are you setting?",
					Required:    true,
					Choices: []adapter.SlashChoice{
						{Name: "Punisher — can punish/release", Value: "punisher"},
						{Name: "Victim — can be punished", Value: "victim"},
						{Name: "Brat — punishment role", Value: "assigned"},
					},
				},
				{
					Type:        adapter.OptionRole,
					Name:        "role",
					Description: "Select a role from the server",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "roles-show",
			Description: "Show configured discipline roles",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "roles-reset",
			Description: "Reset discipline role configuration",
		},
	}
}

// RunManageRoles handles the discipline settings subcommands.
func RunManageRoles(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	switch sub.Name {
	case "roles-set":
		return runRolesSet(ctx, sub)
	case "roles-show":
		return runRolesShow(ctx)
	case "roles-reset":
		return runRolesReset(ctx)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Unknown subcommand.",
		})
	}
}

func subString(sub adapter.SlashArgument, name string) string {
	opt, _ := sub.Option(name)
	return opt.StringValue()
}

func runRolesSet(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	roleType := subString(sub, "type")
	roleID := subString(sub, "role")

	if roleType == "" || roleID == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Missing required options.",
		})
	}

	if err := ctx.Storage.SetPunishRole(ctx.GuildID(), roleType, roleID); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to set %s role: `%v`", roleType, err),
		})
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Set %s role to **%s**.", roleType, roleName(ctx.API, ctx.GuildID(), roleID)),
	})
}

func runRolesShow(ctx *adapter.SlashInteractionContext) error {
	roles := []string{"punisher", "victim", "assigned"}
	var lines []string
	for _, t := range roles {
		rID, _ := ctx.Storage.GetPunishRole(ctx.GuildID(), t)
		if rID != "" {
			if rName, err := ctx.API.RoleName(ctx.GuildID(), rID); err == nil {
				lines = append(lines, fmt.Sprintf("**%s** role set to  %s", t, rName))
			} else {
				lines = append(lines, fmt.Sprintf("**%s**  role set to <@&%s>", t, rID))
			}
		} else {
			lines = append(lines, fmt.Sprintf("**%s** role not set", t))
		}
	}
	return ctx.RespondEphemeral(&adapter.Embed{
		Description: strings.Join(lines, "\n") + "\n\nUse `/settings discipline roles-set` to set or update roles.\n\n Punish is the role that can punish and release people.\nVictim is the role that can be punished.\nAssigned is the punishment role (that is assigned by the punisher).",
	})
}

func runRolesReset(ctx *adapter.SlashInteractionContext) error {
	if err := ctx.Storage.SetPunishRole(ctx.GuildID(), "punisher", ""); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed resetting punisher role: `%v`", err),
		})
	}
	if err := ctx.Storage.SetPunishRole(ctx.GuildID(), "victim", ""); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed resetting victim role: `%v`", err),
		})
	}
	if err := ctx.Storage.SetPunishRole(ctx.GuildID(), "assigned", ""); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed resetting assigned role: `%v`", err),
		})
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Description: "All roles have been reset.",
	})
}
