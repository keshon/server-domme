package command

import (
	"github.com/bwmarrin/discordgo"
)

type Middleware func(Command) Command

type WrappedCommand struct {
	Command
	Wrap func(ctx interface{}) error
}

func (w *WrappedCommand) Run(ctx interface{}) error {
	if w.Wrap != nil {
		return w.Wrap(ctx)
	}
	return w.Command.Run(ctx)
}

func (w *WrappedCommand) Component(ctx *ComponentInteractionContext) error {
	if w.Wrap != nil {
		return w.Wrap(ctx)
	}
	if ch, ok := w.Command.(ComponentInteractionHandler); ok {
		return ch.Component(ctx)
	}
	return nil
}

func (w *WrappedCommand) SlashDefinition() *discordgo.ApplicationCommand {
	if sp, ok := w.Command.(SlashProvider); ok {
		return sp.SlashDefinition()
	}
	return nil
}

func (w *WrappedCommand) ContextDefinition() *discordgo.ApplicationCommand {
	if sp, ok := w.Command.(ContextMenuProvider); ok {
		return sp.ContextDefinition()
	}
	return nil
}

func (w *WrappedCommand) ReactionDefinition() string {
	if sp, ok := w.Command.(ReactionProvider); ok {
		return sp.ReactionDefinition()
	}
	return ""
}

func ApplyMiddlewares(cmd Command, mws ...Middleware) Command {
	for _, mw := range mws {
		cmd = mw(cmd)
	}
	return cmd
}
