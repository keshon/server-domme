package command

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type SetRolesCommand struct{}

func (c *SetRolesCommand) Name() string        { return "set-roles" }
func (c *SetRolesCommand) Description() string { return "Setup special-purpose roles" }
func (c *SetRolesCommand) Aliases() []string   { return []string{} }

func (c *SetRolesCommand) Group() string    { return "core" }
func (c *SetRolesCommand) Category() string { return "⚙️ Settings" }

func (c *SetRolesCommand) RequireAdmin() bool { return true }
func (c *SetRolesCommand) RequireDev() bool   { return false }

func (c *SetRolesCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "Which role are you setting?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Punisher (can punish and release)", Value: "punisher"},
					{Name: "Victim (can be punished)", Value: "victim"},
					{Name: "Brat (punishment role assigned)", Value: "assigned"},
					{Name: "Tasker (can take role based tasks)", Value: "tasker"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionRole,
				Name:        "role",
				Description: "Select a role from the server",
				Required:    true,
			},
		},
	}
}

func (c *SetRolesCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage
	options := event.ApplicationCommandData().Options

	if !isAdministrator(session, event.GuildID, event.Member) {
		return respondEphemeral(session, event, "You must be an Admin to use this command, darling.")
	}

	var roleType, roleID string
	for _, opt := range options {
		switch opt.Name {
		case "type":
			roleType = opt.StringValue()
		case "role":
			roleID = opt.RoleValue(session, event.GuildID).ID
		}
	}

	if roleType == "" || roleID == "" {
		return respondEphemeral(session, event, "Missing parameters. Try again without wasting my time.")
	}

	switch roleType {
	case "tasker":
		err := storage.SetTaskRole(event.GuildID, roleID)
		if err != nil {
			return respondEphemeral(session, event, fmt.Sprintf("Something broke when saving. Error: `%s`", err.Error()))
		}
	default:
		err := storage.SetPunishRole(event.GuildID, roleType, roleID)
		if err != nil {
			return respondEphemeral(session, event, fmt.Sprintf("Something broke when saving. Error: `%s`", err.Error()))
		}
	}

	var response string
	if roleType == "tasker" {
		roleName, err := getRoleNameByID(session, event.GuildID, roleID)
		if err != nil {
			roleName = roleID
		}
		response = fmt.Sprintf("Added **%s** to the list of tasker roles. Update your tasks accordingly.", roleName)
	} else {
		response = fmt.Sprintf("The **%s** role has been updated.", roleType)
	}

	err := respondEphemeral(session, event, response)
	if err != nil {
		return err
	}

	err = logCommand(session, storage, event.GuildID, event.ChannelID, event.Member.User.ID, event.Member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log command:", err)
	}

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

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&SetRolesCommand{},
			),
		),
	)
}
