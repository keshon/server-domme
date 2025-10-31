package middleware

import "server-domme/internal/registry"

// WithGuildOnly wraps a command to enforce guild-only access
func WithGuildOnly() Middleware {
	return func(cmd registry.Command) registry.Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				if v, ok := ctx.(*registry.SlashInteractionContext); ok && v.Event.GuildID == "" {
					return nil
				}
				if v, ok := ctx.(*registry.MessageContext); ok && v.Event.GuildID == "" {
					return nil
				}
				return cmd.Run(ctx)
			},
		}
	}
}
