package middleware

import "server-domme/internal/command"

// WithGuildOnly wraps a command to enforce guild-only access
func WithGuildOnly() command.Middleware {
	return func(cmd command.Command) command.Command {
		return &command.WrappedCommand{
			Command: cmd,
			Wrap: func(ctx interface{}) error {
				if v, ok := ctx.(*command.SlashInteractionContext); ok && v.Event.GuildID == "" {
					return nil
				}
				if v, ok := ctx.(*command.MessageContext); ok && v.Event.GuildID == "" {
					return nil
				}
				return cmd.Run(ctx)
			},
		}
	}
}
