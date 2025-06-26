package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           201,
		Name:           "set-role",
		Description:    "Configure punisher/victim roles (admin-only)",
		Category:       "Admin",
		DCSlashHandler: setRoleHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "Which role are you setting?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Punisher", Value: "punisher"},
					{Name: "Victim", Value: "victim"},
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
				Flags:   1 << 6, // ephemeral
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

	if roleType != "punisher" && roleType != "victim" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Hmm, that's not a valid role type. Try again without embarrassing yourself.",
				Flags:   1 << 6,
			},
		})
		return
	}

	err = storage.SetRoleForGuild(i.GuildID, roleType, roleID)
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

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ The **%s** role has been updated. Good little config, isn't it?", roleType),
			Flags:   1 << 6,
		},
	})
}
