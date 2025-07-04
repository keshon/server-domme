package commands

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           401,
		Name:           "set-role",
		Description:    "Configure punisher/victim/tasker roles",
		Category:       "Administration",
		DCSlashHandler: setRoleHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "Which role are you setting?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Punisher (can punish and release)", Value: "punisher"},
					{Name: "Victim (can be punished)", Value: "victim"},
					{Name: "Brat (punishment role assigned)", Value: "assigned"},
					{Name: "Tasker (can take role based tasks)", Value: "tasker"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionRole,
				Name:        "role",
				Description: "Select a role from the server",
				Required:    true,
			},
		},
	})
}

func setRoleHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.Interaction, ctx.Storage
	options := i.ApplicationCommandData().Options

	member := i.Member
	hasAdmin := false

	guild, err := s.State.Guild(i.GuildID)
	if err != nil || guild == nil {
		guild, err = s.Guild(i.GuildID)
		if err != nil {
			return
		}
	}

	if i.Member.User.ID == guild.OwnerID {
		hasAdmin = true
	} else {
		for _, r := range member.Roles {
			role, _ := s.State.Role(i.GuildID, r)
			if role != nil && role.Permissions&discordgo.PermissionAdministrator != 0 {
				hasAdmin = true
				break
			}
		}
	}

	if !hasAdmin {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You’re not wearing the crown, darling. Only Admins may play God here.",
				Flags:   1 << 6,
			},
		})
		return
	}

	var roleType, roleID string
	for _, opt := range options {
		switch opt.Name {
		case "type":
			roleType = opt.StringValue()
		case "role":
			roleID = opt.RoleValue(s, i.GuildID).ID
		}
	}

	validTypes := map[string]bool{
		"punisher": true,
		"victim":   true,
		"assigned": true,
		"tasker":   true,
	}

	if !validTypes[roleType] {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Hmm, that's not a valid role type. Try again without embarrassing yourself.",
				Flags:   1 << 6,
			},
		})
		return
	}

	if roleType == "tasker" {
		err = storage.SetTaskRole(i.GuildID, roleID)
		if err != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("Something broke when saving, and for once it wasn’t your will. Error: `%s`", err.Error()),
					Flags:   1 << 6,
				},
			})
			return
		}
	} else {
		err = storage.SetPunishRole(i.GuildID, roleType, roleID)
		if err != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("Something broke when saving, and for once it wasn’t your will. Error: `%s`", err.Error()),
					Flags:   1 << 6,
				},
			})
			return
		}
	}

	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Something broke when saving, and for once it wasn’t your will. Error: `%s`", err.Error()),
				Flags:   1 << 6,
			},
		})
		return
	}

	var response string
	if roleType == "tasker" {
		roleName, err := getRoleNameByID(s, i.GuildID, roleID)
		if err != nil {
			roleName = roleID
		}
		response = fmt.Sprintf("✅ Added **%s** to the list of tasker roles. Update your tasks accordingly.", roleName)
	} else {
		response = fmt.Sprintf("✅ The **%s** role has been updated.", roleType)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: response,
			Flags:   1 << 6,
		},
	})

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "set-role")
	if err != nil {
		log.Println("Failed to log command:", err)
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
