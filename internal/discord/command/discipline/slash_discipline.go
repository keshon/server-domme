package discipline

import (
	"fmt"
	"math/rand"
	"slices"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

type DisciplineCommand struct{}

func (c *DisciplineCommand) Name() string        { return "discipline" }
func (c *DisciplineCommand) Description() string { return "Punish or release a brat" }
func (c *DisciplineCommand) Group() string       { return "discipline" }
func (c *DisciplineCommand) Category() string    { return "🎭 Roleplay" }
func (c *DisciplineCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *DisciplineCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: "Punish or release a brat",
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionSubCommand,
				Name:        "punish",
				Description: "Assign the brat role",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionUser,
						Name:        "target",
						Description: "The brat who needs correction",
						Required:    true,
					},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "release",
				Description: "Remove the brat role",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionUser,
						Name:        "target",
						Description: "The brat to be released",
						Required:    true,
					},
				},
			},
		},
	}
}

func (c *DisciplineCommand) Run(ctx *adapter.SlashInteractionContext) error {
	sub, ok := ctx.FirstOption()
	if !ok {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No subcommand provided.",
		})
	}

	targetOpt, _ := sub.Option("target")
	targetID := targetOpt.StringValue()

	switch sub.Name {
	case "punish":
		return c.runPunish(ctx, targetID)
	case "release":
		return c.runRelease(ctx, targetID)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Unknown subcommand.",
		})
	}
}

func (c *DisciplineCommand) runPunish(ctx *adapter.SlashInteractionContext, targetID string) error {
	// Guard the target, not the invoker: PROTECTED_USERS names people who may
	// not be punished.
	cfg := ctx.Config
	if cfg != nil && slices.Contains(cfg.ProtectedUsers, targetID) {
		return ctx.RespondWith(adapter.Reply{Text: "I may be cruel, but I won’t punish the architect of my existence. Creator protected, no whipping allowed. 🙅‍♀️"})
	}

	store := ctx.Storage
	punisherRoleID, _ := store.GetPunishRole(ctx.GuildID(), "punisher")
	victimRoleID, _ := store.GetPunishRole(ctx.GuildID(), "victim")
	assignedRoleID, _ := store.GetPunishRole(ctx.GuildID(), "assigned")

	if punisherRoleID == "" || victimRoleID == "" || assignedRoleID == "" {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "Roles not configured properly. Set them first via `/settings discipline roles-set`.",
		})
		return nil
	}

	if !slices.Contains(ctx.Invoker.Roles, punisherRoleID) {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "Nice try, sugar. You don’t wear the right collar to give punishments.",
		})
		return nil
	}

	if err := ctx.API.AddMemberRole(ctx.GuildID(), targetID, assignedRoleID); err != nil {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to assign role: `%v`", err),
		})
		return nil
	}

	phrase := punishPhrases[rand.Intn(len(punishPhrases))]
	return ctx.RespondWith(adapter.Reply{Text: fmt.Sprintf(phrase, targetID)})
}

func (c *DisciplineCommand) runRelease(ctx *adapter.SlashInteractionContext, targetID string) error {
	store := ctx.Storage
	punisherRoleID, _ := store.GetPunishRole(ctx.GuildID(), "punisher")
	assignedRoleID, _ := store.GetPunishRole(ctx.GuildID(), "assigned")

	if punisherRoleID == "" || assignedRoleID == "" {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "Roles not configured properly. Set them first via `/settings discipline roles-set`.",
		})
		return nil
	}

	if !slices.Contains(ctx.Invoker.Roles, punisherRoleID) {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "No, no, no. You don’t *get* to undo what the real dommes do. Back to your corner.",
		})
		return nil
	}

	if err := ctx.API.RemoveMemberRole(ctx.GuildID(), targetID, assignedRoleID); err != nil {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to remove role: `%v`", err),
		})
		return nil
	}

	return ctx.Respond(&adapter.Embed{
		Description: fmt.Sprintf("🔓 <@%s> has been released. Let's see if they behave.", targetID),
	})
}

func roleName(api adapter.SessionAPI, guildID, roleID string) string {
	if api == nil {
		return roleID
	}
	if name, err := api.RoleName(guildID, roleID); err == nil {
		return name
	}
	return roleID
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
