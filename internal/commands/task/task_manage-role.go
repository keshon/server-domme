package task

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

func (c *TaskCommand) runManageRoles(s *discordgo.Session, e *discordgo.InteractionCreate, storage *storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	if !core.IsAdministrator(s, e.Member) {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "You must be an admin to use this command.",
		})
	}

	switch sub.Name {

	case "set":
		var roleID string
		for _, opt := range sub.Options {
			if opt.Name == "role" {
				role := opt.RoleValue(s, e.GuildID)
				if role != nil {
					roleID = role.ID
				}
			}
		}

		if roleID == "" {
			return core.RespondEphemeral(s, e, "Missing `role` parameter.")
		}

		if err := storage.SetTaskRole(e.GuildID, roleID); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed saving Tasker role: `%s`", err))
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		core.RespondEphemeral(s, e, fmt.Sprintf("✅ Tasker role set to **%s**.", roleName))
		return nil

	case "list":
		roleID, err := storage.GetTaskRole(e.GuildID)
		if err != nil || roleID == "" {
			return core.RespondEphemeral(s, e, "No Tasker role configured yet.")
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		core.RespondEphemeral(s, e, fmt.Sprintf("🧩 Current Tasker role: **%s**", roleName))
		return nil

	case "reset":
		if err := storage.SetTaskRole(e.GuildID, ""); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed resetting Tasker role: %v", err))
		}

		core.RespondEphemeral(s, e, "🗑️ Tasker role has been reset.")
		return nil

	default:
		return core.RespondEphemeral(s, e, fmt.Sprintf("Unknown manage roles subcommand: %s", sub.Name))
	}
}

func getRoleNameByID(s *discordgo.Session, guildID, roleID string) (string, error) {
	guild, err := s.State.Guild(guildID)
	if err != nil || guild == nil {
		guild, err = s.Guild(guildID)
		if err != nil {
			return "", fmt.Errorf("failed to fetch guild: %w", err)
		}
	}
	for _, role := range guild.Roles {
		if role.ID == roleID {
			return role.Name, nil
		}
	}
	return "", fmt.Errorf("role ID %s not found in guild %s", roleID, guildID)
}
