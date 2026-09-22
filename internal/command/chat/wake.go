package chat

import (
	"github.com/bwmarrin/discordgo"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
)

// subWake is /chat wake.
const subWake = "wake"

// wakeOption is /chat wake: wake her when she is asleep or away.
func wakeOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionSubCommand,
		Name:        subWake,
		Description: "Wake her up — early, and she knows it. One body: it wakes her on every server she is on",
	}
}

// runWake wakes her. Being woken is an event with consequences, not a
// switch: her tiredness stays where it was, so she is up for an hour or
// while someone keeps her talking, then goes back to bed if her body was
// not done; and she is told she was woken early.
func (c *ChatCommand) runWake(context *cmdadapter.SlashInteractionContext) error {
	s, e := context.Session, context.Event
	switch c.Service.Wake() {
	case chatsvc.WakeNoBody:
		return respond(s, e, "She has no body switched on (`CHAT_BODY`), so she never sleeps — she is always around.")
	case chatsvc.WakeAwake:
		return respond(s, e, "She is already awake.")
	case chatsvc.WakeBack:
		return respond(s, e, "She was away; she is back now, and will get to what she missed.")
	default:
		return respond(s, e, "She is up — woken early, and she knows it. She will get to what she missed, and once things go quiet she will likely go back to sleep.")
	}
}
