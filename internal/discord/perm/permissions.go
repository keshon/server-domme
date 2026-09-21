package perm

import (
	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/config"
)

// IsAdministrator reports whether a member has administrator privileges in a
// guild, or is the configured developer.
//
// The guild is passed rather than read from the member: a member delivered
// with an interaction carries no guild_id, and reading it from there sent
// every real administrator's check to guild "" — a REST call that fails, and
// a refusal. Measured: /welcome template's editor refused every administrator
// who was not the developer. The permissions Discord resolved for the
// interaction are trusted first, since they already account for every role
// and override; the role walk is for a member from anywhere else.
func IsAdministrator(s *discordgo.Session, guildID string, member *discordgo.Member, cfg *config.Config) bool {
	if member == nil || member.User == nil {
		return false
	}
	if cfg != nil && config.IsDeveloper(cfg, member.User.ID) {
		return true
	}
	if member.Permissions&discordgo.PermissionAdministrator != 0 {
		return true
	}
	if guildID == "" {
		guildID = member.GuildID
	}
	if guildID == "" {
		return false
	}

	guild, err := s.State.Guild(guildID)
	if err != nil || guild == nil {
		guild, err = s.Guild(guildID)
		if err != nil || guild == nil {
			return false
		}
	}

	if member.User.ID == guild.OwnerID {
		return true
	}
	for _, roleID := range member.Roles {
		if role, _ := s.State.Role(guild.ID, roleID); role != nil {
			if role.Permissions&discordgo.PermissionAdministrator != 0 {
				return true
			}
		}
	}
	return false
}

// IsDeveloper reports whether a user ID matches the configured developer.
// Delegates to config for a single source of truth.
func IsDeveloper(cfg *config.Config, userID string) bool {
	return config.IsDeveloper(cfg, userID)
}

// CheckBotPermissions reports whether the bot has ManageMessages permission in
// a channel.
func CheckBotPermissions(s *discordgo.Session, channelID string) bool {
	perms, err := s.UserChannelPermissions(s.State.User.ID, channelID)
	if err != nil {
		return false
	}
	return perms&discordgo.PermissionManageMessages != 0
}
