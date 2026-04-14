package command

import (
	"github.com/keshon/commandkit"
	"server-domme/internal/config"
)

func ConfigFromInvocation(inv *commandkit.Invocation) *config.Config {
	if inv == nil || inv.Data == nil {
		return nil
	}

	switch v := inv.Data.(type) {
	case *SlashInteractionContext:
		return v.Config
	case *ComponentInteractionContext:
		return v.Config
	case *MessageReactionContext:
		return v.Config
	case *MessageApplicationCommandContext:
		return v.Config
	case *MessageContext:
		return v.Config
	default:
		return nil
	}
}

// Register registers a Discord command with the commandkit registry and applies middlewares.
func Register(discordCmd Handler, mws ...commandkit.Middleware) {
	c := commandkit.Apply(&Adapter{Cmd: discordCmd}, mws...)
	commandkit.DefaultRegistry.Register(c)
}

// RegisterCommand is kept temporarily for existing packages. It will be removed once
// command registration becomes explicit in `cmd/discord/main.go`.
func RegisterCommand(discordCmd Handler, mws ...commandkit.Middleware) {
	Register(discordCmd, mws...)
}

