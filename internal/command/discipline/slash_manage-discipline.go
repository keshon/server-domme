package discipline

import (
	"fmt"

	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/middleware"
	"server-domme/internal/storage"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type ManageDisciplineCommand struct{}

func (c *ManageDisciplineCommand) Name() string        { return "manage-discipline" }
func (c *ManageDisciplineCommand) Description() string { return "Discipline settings" }
func (c *ManageDisciplineCommand) Group() string       { return "discipline" }
func (c *ManageDisciplineCommand) Category() string    { return "⚙️ Settings" }
func (c *ManageDisciplineCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *ManageDisciplineCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
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
				Description: "List all configured discipline roles",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-roles",
				Description: "Reset all discipline role configurations",
			},
		},
	}
}

func (c *ManageDisciplineCommand) Run(ctx interface{}) error {
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
	return c.runManageRoles(s, e, *storage, sub)
}

func (c *ManageDisciplineCommand) runManageRoles(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {

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
			return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Missing required options.",
			})
		}

		if err := storage.SetPunishRole(e.GuildID, roleType, roleID); err != nil {
			return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to set %s role: %v", roleType, err),
			})
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Set %s role to **%s**.", roleType, roleName),
		})
		return nil

	case "list-roles":
		roles := []string{"punisher", "victim", "assigned"}
		var lines []string
		for _, t := range roles {
			rID, _ := storage.GetPunishRole(e.GuildID, t)
			if rID != "" {
				if rName, err := getRoleNameByID(s, e.GuildID, rID); err == nil {
					lines = append(lines, fmt.Sprintf("**%s** role set to  %s", t, rName))
				} else {
					lines = append(lines, fmt.Sprintf("**%s**  role set to <@&%s>", t, rID))
				}
			} else {
				lines = append(lines, fmt.Sprintf("**%s** role not set", t))
			}
		}
		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: strings.Join(lines, "\n") + "\n\nUse `/manage-discipline set-roles` to set or update roles.\n\n Punish is the role that can punish and release people.\nVictim is the role that can be punished.\nAssigned is the punishment role (that is assigned by the punisher).",
		})
		return nil

	case "reset-roles":
		if err := storage.SetPunishRole(e.GuildID, "punisher", ""); err != nil {
			return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed resetting punisher role: %v", err),
			})
		}
		if err := storage.SetPunishRole(e.GuildID, "victim", ""); err != nil {
			return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed resetting victim role: %v", err),
			})
		}
		if err := storage.SetPunishRole(e.GuildID, "assigned", ""); err != nil {
			return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed resetting assigned role: %v", err),
			})
		}

		bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "All roles have been reset.",
		})
		return nil
	}

	return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Unknown subcommand.",
	})
}

func init() {
	command.RegisterCommand(
		command.ApplyMiddlewares(
			&ManageDisciplineCommand{},
			middleware.WithGroupAccessCheck(),
			middleware.WithGuildOnly(),
			middleware.WithUserPermissionCheck(),
			middleware.WithCommandLogger(),
		),
	)
}
