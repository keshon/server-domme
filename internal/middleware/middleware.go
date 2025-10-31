package middleware

import (
	"server-domme/internal/registry"

	"github.com/bwmarrin/discordgo"
)

type Middleware func(registry.Command) registry.Command

type wrappedCommand struct {
	registry.Command
	wrap func(ctx interface{}) error
}

func (w *wrappedCommand) Run(ctx interface{}) error {
	if w.wrap != nil {
		return w.wrap(ctx)
	}
	return w.Command.Run(ctx)
}

func (w *wrappedCommand) Component(ctx *registry.ComponentInteractionContext) error {
	if w.wrap != nil {
		return w.wrap(ctx)
	}
	if ch, ok := w.Command.(registry.ComponentInteractionHandler); ok {
		return ch.Component(ctx)
	}
	return nil
}

func (w *wrappedCommand) SlashDefinition() *discordgo.ApplicationCommand {
	if sp, ok := w.Command.(registry.SlashProvider); ok {
		return sp.SlashDefinition()
	}
	return nil
}

func (w *wrappedCommand) ContextDefinition() *discordgo.ApplicationCommand {
	if sp, ok := w.Command.(registry.ContextMenuProvider); ok {
		return sp.ContextDefinition()
	}
	return nil
}

func (w *wrappedCommand) ReactionDefinition() string {
	if sp, ok := w.Command.(registry.ReactionProvider); ok {
		return sp.ReactionDefinition()
	}
	return ""
}

func ApplyMiddlewares(cmd registry.Command, mws ...Middleware) registry.Command {
	for _, mw := range mws {
		cmd = mw(cmd)
	}
	return cmd
}
