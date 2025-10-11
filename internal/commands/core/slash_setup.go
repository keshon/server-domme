package core

import (
	"fmt"
	"log"
	"server-domme/internal/core"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type SetupCommand struct{}

func (c *SetupCommand) Name() string        { return "setup" }
func (c *SetupCommand) Description() string { return "Setup server roles and channels" }
func (c *SetupCommand) Group() string       { return "core" }
func (c *SetupCommand) Category() string    { return "⚙️ Settings" }
func (c *SetupCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *SetupCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "channels",
				Description: "Setup special-purpose channels",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "type",
						Description: "Which type of channel?",
						Required:    true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "Confession Channel", Value: "confession"},
							{Name: "Announcement Channel", Value: "announce"},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionChannel,
						Name:        "channel",
						Description: "Pick a channel from this server",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "roles",
				Description: "Setup special-purpose roles",
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
							{Name: "Tasker — can take tasks", Value: "tasker"},
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
		},
	}
}

func (c *SetupCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	if err := core.RespondDeferredEphemeral(session, event); err != nil {
		log.Printf("[ERROR] Failed to defer interaction: %v", err)
		return err
	}

	options := event.ApplicationCommandData().Options
	if len(options) == 0 {
		return core.FollowupEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := options[0]
	switch sub.Name {
	case "channels":
		return runSetupChannels(session, event, *storage, sub)
	case "roles":
		return runSetupRoles(session, event, *storage, sub)
	default:
		return core.FollowupEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

func runSetupChannels(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	var kind, channelID string
	for _, opt := range sub.Options {
		switch opt.Name {
		case "type":
			kind = opt.StringValue()
		case "channel":
			channelID = opt.ChannelValue(s).ID
		}
	}

	if kind == "" || channelID == "" {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Missing parameters.",
		})
	}

	if err := storage.SetConfessChannel(e.GuildID, channelID); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to set channel: ```%v```", err),
		})
	}

	msg := map[string]string{
		"confession": "Confession channel updated.",
		"announce":   "Announcement channel set.",
	}[kind]
	if msg == "" {
		msg = fmt.Sprintf("✅ Channel for `%s` set.", kind)
	}

	return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: msg,
	})
}

func runSetupRoles(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
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
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Missing parameters.",
		})
	}

	switch roleType {
	case "tasker":
		if err := storage.SetTaskRole(e.GuildID, roleID); err != nil {
			return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed saving tasker role: `%s`", err.Error()),
			})
		}
	default:
		if err := storage.SetPunishRole(e.GuildID, roleType, roleID); err != nil {
			return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed saving role: `%s`", err.Error()),
			})
		}
	}

	roleName := roleID
	if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
		roleName = rName
	}

	msg := roleName
	if roleType == "tasker" {
		msg = fmt.Sprintf("Added **%s** to the list of tasker roles. Update your tasks accordingly.", roleName)
	} else {
		msg = fmt.Sprintf("The **%s** role has been updated.", roleType)
	}

	return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: msg,
	})
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

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&SetupCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
