package discipline

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

// RunManageDisciplineRoles handles discipline role settings subcommands.
func RunManageDisciplineRoles(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	switch sub.Name {
	case "roles-set":
		var roleType, roleID string
		for _, opt := range sub.Options {
			switch opt.Name {
			case "type":
				roleType = opt.StringValue()
			case "role":
				roleID = opt.RoleValue(s, e.GuildID).ID
			}
		}

		if roleType == "" || roleID == "" {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Missing required options.",
			})
		}

		if err := storage.SetPunishRole(e.GuildID, roleType, roleID); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to set %s role: %v", roleType, err),
			})
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Set %s role to **%s**.", roleType, roleName),
		})
		return nil

	case "roles-show":
		roles := []string{"punisher", "victim", "assigned"}
		var lines []string
		for _, t := range roles {
			rID, _ := storage.GetPunishRole(e.GuildID, t)
			if rID != "" {
				if rName, err := getRoleNameByID(s, e.GuildID, rID); err == nil {
					lines = append(lines, fmt.Sprintf("**%s** role set to  %s", t, rName))
				} else {
					lines = append(lines, fmt.Sprintf("**%s**  role set to <@&%s>", t, rID))
				}
			} else {
				lines = append(lines, fmt.Sprintf("**%s** role not set", t))
			}
		}
		reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: strings.Join(lines, "\n") + "\n\nUse `/settings discipline roles-set` to set or update roles.\n\n Punish is the role that can punish and release people.\nVictim is the role that can be punished.\nAssigned is the punishment role (that is assigned by the punisher).",
		})
		return nil

	case "roles-reset":
		if err := storage.SetPunishRole(e.GuildID, "punisher", ""); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed resetting punisher role: %v", err),
			})
		}
		if err := storage.SetPunishRole(e.GuildID, "victim", ""); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed resetting victim role: %v", err),
			})
		}
		if err := storage.SetPunishRole(e.GuildID, "assigned", ""); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed resetting assigned role: %v", err),
			})
		}

		reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "All roles have been reset.",
		})
		return nil
	}

	return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Unknown subcommand.",
	})
}

// DisciplineRolesOptions returns slash options for discipline role settings.
func DisciplineRolesOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "roles-set",
			Description: "Configure discipline roles",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "type",
					Description: "Which role are you setting?",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Punisher — can punish/release", Value: "punisher"},
						{Name: "Victim — can be punished", Value: "victim"},
						{Name: "Brat — punishment role", Value: "assigned"},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Select a role from the server",
					Required:    true,
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "roles-show",
			Description: "Show configured discipline roles",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "roles-reset",
			Description: "Reset discipline role configuration",
		},
	}
}
