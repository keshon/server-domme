package core

import (
	"log"
	"server-domme/internal/config"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

const EmbedColor = 0xb01e66

func Respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	})
	return err
}

func RespondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	return err
}

func MessageRespond(s *discordgo.Session, channelID string, content string) error {
	_, err := s.ChannelMessageSend(channelID, content)
	return err
}

func LogCommand(s *discordgo.Session, storage *storage.Storage, guildID, channelID, userID, username, commandName string) error {
	channel, err := s.State.Channel(channelID)
	if err != nil {
		channel, err = s.Channel(channelID)
		if err != nil {
			log.Println("Failed to fetch channel:", err)
		}
	}
	channelName := ""
	if channel != nil {
		channelName = channel.Name
	}

	guild, err := s.State.Guild(guildID)
	if err != nil {
		guild, err = s.Guild(guildID)
		if err != nil {
			log.Println("Failed to fetch guild:", err)
		}
	}
	guildName := ""
	if guild != nil {
		guildName = guild.Name
	}

	return storage.SetCommand(
		guildID,
		channelID,
		channelName,
		guildName,
		userID,
		username,
		commandName,
	)
}

func IsAdministrator(s *discordgo.Session, guildID string, member *discordgo.Member) bool {
	cfg := config.New()
	if member.User.ID == cfg.DeveloperID {
		return true
	}

	guild, err := s.State.Guild(guildID)
	if err != nil || guild == nil {
		guild, err = s.Guild(guildID)
		if err != nil {
			return false
		}
	}

	if member.User.ID == guild.OwnerID {
		return true
	}

	for _, r := range member.Roles {
		role, _ := s.State.Role(guildID, r)
		if role != nil && role.Permissions&discordgo.PermissionAdministrator != 0 {
			return true
		}
	}

	return false
}

func IsDeveloper(userID string) bool {
	cfg := config.New()
	return userID == cfg.DeveloperID
}

func CheckBotPermissions(s *discordgo.Session, channelID string) bool {
	botID := s.State.User.ID
	perms, err := s.UserChannelPermissions(botID, channelID)
	if err != nil {
		return false
	}
	return perms&discordgo.PermissionManageMessages != 0
}

func CheckGroupAccess(c Command, guildID string, s *storage.Storage, respond func(string)) bool {
	group := c.Group()
	if group == "" {
		return true
	}
	disabled, err := s.IsGroupDisabled(guildID, group)
	if err != nil {
		return true
	}
	if disabled {
		respond("Эта группа команд отключена на этом сервере.")
		return false
	}
	return true
}
