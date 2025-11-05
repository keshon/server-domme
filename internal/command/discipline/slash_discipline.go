package discipline

import (
	"fmt"
	"math/rand"
	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/config"
	"server-domme/internal/middleware"

	"server-domme/internal/storage"
	"slices"

	"github.com/bwmarrin/discordgo"
)

type DisciplineCommand struct{}

func (c *DisciplineCommand) Name() string        { return "discipline" }
func (c *DisciplineCommand) Description() string { return "Punish or release a brat" }
func (c *DisciplineCommand) Group() string       { return "discipline" }
func (c *DisciplineCommand) Category() string    { return "🎭 Roleplay" }
func (c *DisciplineCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *DisciplineCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: "Punish or release a brat.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "punish",
				Description: "Assign the brat role",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "target",
						Description: "The brat who needs correction",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "release",
				Description: "Remove the brat role",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "target",
						Description: "The brat to be released",
						Required:    true,
					},
				},
			},
		},
	}
}

func (c *DisciplineCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	storage := context.Storage

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := data.Options[0]
	targetID := sub.Options[0].UserValue(nil).ID

	switch sub.Name {
	case "punish":
		return c.runPunish(s, e, *storage, targetID)
	case "release":
		return c.runRelease(s, e, *storage, targetID)
	default:
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Unknown subcommand.",
		})
	}
}

func (c *DisciplineCommand) runPunish(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, targetID string) error {
	cfg := config.New()
	if slices.Contains(cfg.ProtectedUsers, e.Member.User.ID) {
		bot.Respond(s, e, "I may be cruel, but I won’t punish the architect of my existence. Creator protected, no whipping allowed. 🙅‍♀️")
		return nil
	}

	punisherRoleID, _ := storage.GetPunishRole(e.GuildID, "punisher")
	victimRoleID, _ := storage.GetPunishRole(e.GuildID, "victim")
	assignedRoleID, _ := storage.GetPunishRole(e.GuildID, "assigned")

	if punisherRoleID == "" || victimRoleID == "" || assignedRoleID == "" {
		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Roles not configured properly. Set them first via `/manage-discipline roles`.",
		})
		return nil
	}

	if !slices.Contains(e.Member.Roles, punisherRoleID) {
		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Nice try, sugar. You don’t wear the right collar to give punishments.",
		})
		return nil
	}

	err := s.GuildMemberRoleAdd(e.GuildID, targetID, assignedRoleID)
	if err != nil {
		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to assign role: %v", err),
		})
		return nil
	}

	phrase := punishPhrases[rand.Intn(len(punishPhrases))]
	bot.Respond(s, e, fmt.Sprintf(phrase, targetID))
	return nil
}

func (c *DisciplineCommand) runRelease(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, targetID string) error {
	punisherRoleID, _ := storage.GetPunishRole(e.GuildID, "punisher")
	assignedRoleID, _ := storage.GetPunishRole(e.GuildID, "assigned")

	if punisherRoleID == "" || assignedRoleID == "" {
		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Roles not configured properly. Set them first via `/manage-discipline roles`.",
		})
		return nil
	}

	if !slices.Contains(e.Member.Roles, punisherRoleID) {
		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No, no, no. You don’t *get* to undo what the real dommes do. Back to your corner.",
		})
		return nil
	}

	err := s.GuildMemberRoleRemove(e.GuildID, targetID, assignedRoleID)
	if err != nil {
		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to remove role: %v", err),
		})
		return nil
	}

	bot.RespondEmbed(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("🔓 <@%s> has been released. Let's see if they behave.", targetID),
	})
	return nil
}

func getRoleNameByID(s *discordgo.Session, guildID, roleID string) (string, error) {
	guild, err := s.State.Guild(guildID)
	if err != nil || guild == nil {
		guild, err = s.Guild(guildID)
		if err != nil {
			return "", fmt.Errorf("failed to fetch guild: %w", err)
		}
	}
	for _, role := range guild.Roles {
		if role.ID == roleID {
			return role.Name, nil
		}
	}
	return "", fmt.Errorf("role ID %s not found in guild %s", roleID, guildID)
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

func init() {
	command.RegisterCommand(
		command.ApplyMiddlewares(
			&DisciplineCommand{},
			middleware.WithGroupAccessCheck(),
			middleware.WithGuildOnly(),
			middleware.WithUserPermissionCheck(),
			middleware.WithCommandLogger(),
		),
	)
}
