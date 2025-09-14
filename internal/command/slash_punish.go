package command

import (
	"fmt"
	"log"
	"math/rand"
	"server-domme/internal/config"
	"server-domme/internal/core"
	"slices"

	"github.com/bwmarrin/discordgo"
)

type PunishCommand struct{}

func (c *PunishCommand) Name() string        { return "punish" }
func (c *PunishCommand) Description() string { return "Assign the brat role" }
func (c *PunishCommand) Aliases() []string   { return []string{} }
func (c *PunishCommand) Group() string       { return "punish" }
func (c *PunishCommand) Category() string    { return "🎭 Roleplay" }
func (c *PunishCommand) RequireAdmin() bool  { return false }
func (c *PunishCommand) RequireDev() bool    { return false }

func (c *PunishCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "target",
				Description: "The brat who needs correction",
				Required:    true,
			},
		},
	}
}

func (c *PunishCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*core.SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

	cfg := config.New()
	if slices.Contains(cfg.ProtectedUsers, event.Member.User.ID) {
		core.Respond(session, event, "I may be cruel, but I won’t punish the architect of my existence. Creator protected, no whipping allowed. 🙅‍♀️")
		return nil
	}

	punisherRoleID, _ := storage.GetPunishRole(event.GuildID, "punisher")
	victimRoleID, _ := storage.GetPunishRole(event.GuildID, "victim")
	assignedRoleID, _ := storage.GetPunishRole(event.GuildID, "assigned")

	if punisherRoleID == "" || victimRoleID == "" || assignedRoleID == "" {
		core.RespondEphemeral(session, event, "Role setup incomplete. Punisher, victim, and assigned roles must be configured.")
		return nil
	}

	if !slices.Contains(event.Member.Roles, punisherRoleID) {
		core.RespondEphemeral(session, event, "Nice try, sugar. You don’t wear the right collar to give punishments.")
		return nil
	}

	var targetID string
	for _, opt := range event.ApplicationCommandData().Options {
		if opt.Name == "target" {
			targetID = opt.Value.(string)
		}
	}

	if targetID == "" {
		core.RespondEphemeral(session, event, "No brat selected? A Domme without a target? Unthinkable.")
		return nil
	}

	err := session.GuildMemberRoleAdd(event.GuildID, targetID, assignedRoleID)
	if err != nil {
		core.RespondEphemeral(session, event, fmt.Sprintf("Tried to punish them, but they squirmed away: ```%v```", err))
		return nil
	}

	phrase := punishPhrases[rand.Intn(len(punishPhrases))]
	core.Respond(session, event, fmt.Sprintf(phrase, targetID))

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&PunishCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}

var punishPhrases = []string{
	"🔒 <@%s> has been sent to the Brat Corner. Someone finally found the line and crossed it.",
	"🚷 <@%s> has been escorted to the Brat Corner—with attitude still intact, unfortunately.",
	"🪑 <@%s> is now in time-out. Yes, again. No, we’re not negotiating. Enjoy the Brat Corner.",
	"📢 <@%s> has been silenced with sass and relocated to the Brat Corner.",
	"🧼 <@%s>'s mouth was too dirty. Sent to scrub up in the Brat Corner.",
	"📦 <@%s> has been packaged and shipped directly to the Brat Corner. No returns.",
	"🫣 <@%s> thought they were cute. The Brat Corner says otherwise.",
	"🥇 <@%s> won gold in the Olympic sport of testing my patience. Your medal ceremony is in the Brat Corner.",
	"🎭 <@%s> put on quite the performance... now take your bow in the Brat Corner.",
	"🚨 <@%s> triggered the ‘Too Much Mouth’ alarm. Detained in the Brat Corner.",
	"🛑 <@%s>, you’ve reached your limit. Off to the Brat Corner you go.",
	"🔇 <@%s> has been muted by the Ministry of Domme Affairs. Brat Corner is your next stop.",
	"🫦 <@%s> bit off more than they could brat. Assigned to the Brat Corner.",
	"🧂 <@%s> was too salty to handle. Now marinating in the Brat Corner.",
	"🎯 <@%s> made themselves a target. Direct hit—Brat Corner, no detour.",
	"💅 <@%s> was serving attitude. Now serving time. In the Brat Corner.",
	"🍑 <@%s>'s behavior? Spanked metaphorically. Then marched to the Brat Corner.",
	"🕰️ <@%s> needed a time-out. Brat Corner is booked just for you.",
	"📉 <@%s>'s respect levels dropped below tolerable. Brat Corner is the only solution.",
	"👶 <@%s> cried ‘unfair.’ Aww. The Brat Corner has tissues and regret.",
	"🍵 <@%s> spilled too much tea and not enough sense. Steeping now in the Brat Corner.",
	"📖 <@%s>, your brat chapter just ended. The Brat Corner is your epilogue.",
	"🥄 <@%s> stirred too much. Sent to simmer in the Brat Corner.",
	"🎀 <@%s> looked cute doing wrong. Now look cute in the Brat Corner.",
	"🧯 <@%s> got too hot to handle. Cooled off directly in the Brat Corner.",
	"📸 <@%s> caught in 4K acting up. Evidence archived. Brat Corner sentence executed.",
	"🫥 <@%s> vanished from good graces. Brat Corner is their new mailing address.",
	"🎲 <@%s> gambled with attitude and lost. Brat Corner is the house that always wins.",
	"📌 <@%s> has been pinned for public shaming. Displayed proudly in the Brat Corner.",
	"🕳️ <@%s>, dig yourself out—if you can. The Brat Corner has depth and no rope.",
	"🛋️ <@%s> is now grounded. In the Brat Corner. Permanently.",
	"📺 <@%s> is now broadcasting live... from the Brat Corner. Audience: none.",
	"🪤 <@%s> walked right into it. The trap was the Brat Corner all along.",
	"📎 <@%s> has been attached to the Brat Report. Filed permanently in the Brat Corner.",
}
