package chat

import (
	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
)

// /chat reflect and its option.
const (
	subReflect = "reflect"
	optToday   = "today"
)

// reflectOption is /chat reflect: have her look back now instead of in the
// early morning.
func reflectOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionSubCommand,
		Name:        subReflect,
		Description: "Have her look back now on the days she has not made sense of yet",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        optToday,
				Description: "Today so far as well; the night still looks back on the whole day",
			},
		},
	}
}

// runReflect queues the reflection and answers at once: reflecting is
// several model calls, far past the three seconds Discord waits for an
// answer, and the reflection loop is what owns it.
func (c *ChatCommand) runReflect(context *cmdadapter.SlashInteractionContext, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	s, e := context.Session, context.Event
	today := false
	for _, opt := range sub.Options {
		if opt.Name == optToday {
			today = opt.BoolValue()
		}
	}
	if !c.Service.ReflectNow(e.GuildID, today) {
		return respond(s, e, "She is already looking back. Give it a minute.")
	}
	msg := "She is looking back on the days she has not made sense of yet"
	if today {
		msg += ", and on today so far"
	}
	msg += ". It takes a minute or two; `/chat status` and `/chat about` show what came of it."
	return respond(s, e, msg)
}
