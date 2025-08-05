package command

import "github.com/bwmarrin/discordgo"

type Middleware func(Command) Command

func WithGuildOnly(cmd Command) Command {
	return &wrappedCommand{
		Command: cmd,
		wrap: func(ctx interface{}) error {
			switch v := ctx.(type) {
			case *SlashContext:
				if v.Event.GuildID == "" {
					_ = v.Session.InteractionRespond(v.Event.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: "You must be in a guild to use this command.",
							Flags:   discordgo.MessageFlagsEphemeral,
						},
					})
					return nil
				}
			}
			return cmd.Run(ctx)
		},
	}
}

type wrappedCommand struct {
	Command
	wrap func(ctx interface{}) error
}

func (w *wrappedCommand) Run(ctx interface{}) error {
	return w.wrap(ctx)
}
