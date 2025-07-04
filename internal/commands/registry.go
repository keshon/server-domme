// /internal/commands/registry.go
package commands

import (
	"log"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type Command struct {
	Sort        int
	Name        string
	Aliases     []string
	Description string
	Category    string

	DCSlashHandler     func(ctx *SlashContext)
	SlashOptions       []*discordgo.ApplicationCommandOption
	DCComponentHandler func(*ComponentContext)
}

var commandRegistry = map[string]*Command{}

func Register(cmd *Command) {
	commandRegistry[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		commandRegistry[alias] = cmd
	}
}

func Get(name string) (*Command, bool) {
	cmd, ok := commandRegistry[name]
	return cmd, ok
}

func All() []*Command {
	var list []*Command
	seen := make(map[string]bool)
	for _, cmd := range commandRegistry {
		if !seen[cmd.Name] {
			list = append(list, cmd)
			seen[cmd.Name] = true
		}
	}
	return list
}

func logCommand(s *discordgo.Session, storage *storage.Storage, guildID, channelID, userID, username, commandName string) error {
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
