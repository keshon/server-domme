package discipline

import (
	"fmt"
	"math/rand"
	"server-domme/internal/config"
	"server-domme/internal/core"
	"server-domme/internal/storage"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type DisciplineCommand struct{}

func (c *DisciplineCommand) Name() string { return "discipline" }
func (c *DisciplineCommand) Description() string {
	return "Punish or release a brat, or manage discipline roles"
}
func (c *DisciplineCommand) Group() string    { return "discipline" }
func (c *DisciplineCommand) Category() string { return "🎭 Roleplay" }
func (c *DisciplineCommand) UserPermissions() []int64 {
	return []int64{}
}

// ----- Slash Definition -----
func (c *DisciplineCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
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
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "manage",
				Description: "Manage discipline roles",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "set-roles",
						Description: "Set or update discipline roles",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionString,
								Name:        "type",
								Description: "Which role are you setting?",
								Required:    true,
								Choices: []*discordgo.ApplicationCommandOptionChoice{
									{Name: "Punisher — can punish/release", Value: "punisher"},
									{Name: "Victim — can be punished", Value: "victim"},
									{Name: "Brat — punishment role", Value: "assigned"},
								},
							},
							{
								Type:        discordgo.ApplicationCommandOptionRole,
								Name:        "role",
								Description: "Select a role from the server",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "list-roles",
						Description: "List all currently configured discipline roles",
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "reset-roles",
						Description: "Reset all discipline role configurations",
					},
				},
			},
		},
	}
}

func (c *DisciplineCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	if len(event.ApplicationCommandData().Options) == 0 {
		core.RespondEphemeral(session, event, "No subcommand provided.")
		return nil
	}

	first := event.ApplicationCommandData().Options[0]

	switch first.Type {
	case discordgo.ApplicationCommandOptionSubCommand:
		targetID := first.Options[0].UserValue(nil).ID
		switch first.Name {
		case "punish":
			return runPunish(session, event, *storage, targetID)
		case "release":
			return runRelease(session, event, *storage, targetID)
		default:
			core.RespondEphemeral(session, event, "Unknown action.")
		}
	case discordgo.ApplicationCommandOptionSubCommandGroup:
		if first.Name == "manage" && len(first.Options) > 0 {
			sub := first.Options[0]
			return runManageRoles(session, event, *storage, sub)
		}
	default:
		core.RespondEphemeral(session, event, "Unknown command structure.")
	}

	return nil
}

func runPunish(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, targetID string) error {
	cfg := config.New()
	if slices.Contains(cfg.ProtectedUsers, e.Member.User.ID) {
		core.Respond(s, e, "I may be cruel, but I won’t punish the architect of my existence. Creator protected, no whipping allowed. 🙅‍♀️")
		return nil
	}

	punisherRoleID, _ := storage.GetPunishRole(e.GuildID, "punisher")
	victimRoleID, _ := storage.GetPunishRole(e.GuildID, "victim")
	assignedRoleID, _ := storage.GetPunishRole(e.GuildID, "assigned")

	if punisherRoleID == "" || victimRoleID == "" || assignedRoleID == "" {
		core.RespondEphemeral(s, e, "Role setup incomplete. Punisher, victim, and assigned roles must be configured.")
		return nil
	}

	if !slices.Contains(e.Member.Roles, punisherRoleID) {
		core.RespondEphemeral(s, e, "Nice try, sugar. You don’t wear the right collar to give punishments.")
		return nil
	}

	err := s.GuildMemberRoleAdd(e.GuildID, targetID, assignedRoleID)
	if err != nil {
		core.RespondEphemeral(s, e, fmt.Sprintf("Tried to punish them, but they squirmed away: ```%v```", err))
		return nil
	}

	phrase := punishPhrases[rand.Intn(len(punishPhrases))]
	core.Respond(s, e, fmt.Sprintf(phrase, targetID))
	return nil
}

func runRelease(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, targetID string) error {
	punisherRoleID, _ := storage.GetPunishRole(e.GuildID, "punisher")
	assignedRoleID, _ := storage.GetPunishRole(e.GuildID, "assigned")

	if punisherRoleID == "" || assignedRoleID == "" {
		core.RespondEphemeral(s, e, "Roles not configured properly. Set them first via `/discipline manage roles`.")
		return nil
	}

	if !slices.Contains(e.Member.Roles, punisherRoleID) {
		core.RespondEphemeral(s, e, "No, no, no. You don’t *get* to undo what the real dommes do. Back to your corner.")
		return nil
	}

	err := s.GuildMemberRoleRemove(e.GuildID, targetID, assignedRoleID)
	if err != nil {
		core.RespondEphemeral(s, e, fmt.Sprintf("Tried to undo their sentence, but the chains are tight: ```%v```", err))
		return nil
	}

	core.Respond(s, e, fmt.Sprintf("🔓 <@%s> has been released. Let's see if they behave. Doubt it.", targetID))
	return nil
}

func runManageRoles(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	if !core.IsAdministrator(s, e.Member) {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{Description: "You must be an admin to use this command."})
	}

	switch sub.Name {
	case "set-roles":
		var roleType, roleID string
		for _, opt := range sub.Options {
			switch opt.Name {
			case "type":
				roleType = opt.StringValue()
			case "role":
				roleID = opt.RoleValue(s, e.GuildID).ID
			}
		}

		if roleType == "" || roleID == "" {
			return core.RespondEphemeral(s, e, "Missing parameters.")
		}

		if err := storage.SetPunishRole(e.GuildID, roleType, roleID); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed saving role: `%s`", err))
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		core.RespondEphemeral(s, e, fmt.Sprintf("The **%s** role has been updated to **%s**.", roleType, roleName))
		return nil

	case "list-roles":
		roles := []string{"punisher", "victim", "assigned"}
		var lines []string
		for _, t := range roles {
			rID, _ := storage.GetPunishRole(e.GuildID, t)
			if rID != "" {
				if rName, err := getRoleNameByID(s, e.GuildID, rID); err == nil {
					lines = append(lines, fmt.Sprintf("**%s** → %s", t, rName))
				} else {
					lines = append(lines, fmt.Sprintf("**%s** → <@&%s>", t, rID))
				}
			} else {
				lines = append(lines, fmt.Sprintf("**%s** → not set", t))
			}
		}
		core.RespondEphemeral(s, e, strings.Join(lines, "\n"))
		return nil

	case "reset-roles":
		if err := storage.SetPunishRole(e.GuildID, "punisher", ""); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed resetting punisher role: %v", err))
		}
		if err := storage.SetPunishRole(e.GuildID, "victim", ""); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed resetting victim role: %v", err))
		}
		if err := storage.SetPunishRole(e.GuildID, "assigned", ""); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed resetting assigned role: %v", err))
		}

		core.RespondEphemeral(s, e, "All discipline roles have been reset.")
		return nil
	}

	return core.RespondEphemeral(s, e, fmt.Sprintf("Unknown manage subcommand: %s", sub.Name))
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
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&DisciplineCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
