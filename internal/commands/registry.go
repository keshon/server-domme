// /internal/commands/registry.go
package commands

import (
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
