package core

import "github.com/bwmarrin/discordgo"

type Middleware func(Command) Command

type wrappedCommand struct {
	Command
	wrap func(ctx interface{}) error
}

func (w *wrappedCommand) Run(ctx interface{}) error {
	if w.wrap != nil {
		return w.wrap(ctx)
	}
	return w.Command.Run(ctx)
}

func (w *wrappedCommand) Component(ctx *ComponentInteractionContext) error {
	if w.wrap != nil {
		return w.wrap(ctx)
	}
	if ch, ok := w.Command.(ComponentInteractionHandler); ok {
		return ch.Component(ctx)
	}
	return nil
}

func (w *wrappedCommand) SlashDefinition() *discordgo.ApplicationCommand {
	if sp, ok := w.Command.(SlashProvider); ok {
		return sp.SlashDefinition()
	}
	return nil
}

func (w *wrappedCommand) ContextDefinition() *discordgo.ApplicationCommand {
	if sp, ok := w.Command.(ContextMenuProvider); ok {
		return sp.ContextDefinition()
	}
	return nil
}

func (w *wrappedCommand) ReactionDefinition() string {
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
