package commands

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           410,
		Name:           "set-roles",
		Description:    "Appoint punisher, victim, or tasker roles.",
		Category:       "🏰 Court Administration",
		AdminOnly:      true,
		DCSlashHandler: setRoleSlashHandler,
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

func setRoleSlashHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options

	if !isAdmin(s, i.GuildID, i.Member) {
		respondEphemeral(s, i, "You must be an Admin to use this command, darling.")
		return
	}

	var err error
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
		respondEphemeral(s, i, "Hmm, that's not a valid role type. Try again without embarrassing yourself.")
		return
	}

	if roleType == "tasker" {
		err = storage.SetTaskRole(i.GuildID, roleID)
		if err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Something broke when saving, and for once it wasn’t your will. Error: `%s`", err.Error()))
			return
		}
	} else {
		err = storage.SetPunishRole(i.GuildID, roleType, roleID)
		if err != nil {
			respondEphemeral(s, i, fmt.Sprintf("Something broke when saving, and for once it wasn’t your will. Error: `%s`", err.Error()))
			return
		}
	}

	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Something broke when saving, and for once it wasn’t your will. Error: `%s`", err.Error()))
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

	respondEphemeral(s, i, response)

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "set-roles")
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
