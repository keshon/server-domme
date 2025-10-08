package core

func WithGuildOnly() Middleware {
	return func(cmd Command) Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				if v, ok := ctx.(*SlashInteractionContext); ok && v.Event.GuildID == "" {
					return nil
				}
				if v, ok := ctx.(*MessageContext); ok && v.Event.GuildID == "" {
					return nil
				}
				return cmd.Run(ctx)
			},
		}
	}
}
