package command

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type SetRolesCommand struct{}

func (c *SetRolesCommand) Name() string        { return "set-roles" }
func (c *SetRolesCommand) Description() string { return "Appoint punisher, victim, or tasker roles" }
func (c *SetRolesCommand) Category() string    { return "⚙️ Maintenance" }
func (c *SetRolesCommand) Aliases() []string   { return nil }

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
	slashCtx, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("не тот тип контекста")
	}

	s := slashCtx.Session
	i := slashCtx.Event
	st := slashCtx.Storage
	opts := i.ApplicationCommandData().Options

	if !isAdministrator(s, i.GuildID, i.Member) {
		return respondEphemeral(s, i, "You must be an Admin to use this command, darling.")
	}

	var roleType, roleID string
	for _, opt := range opts {
		switch opt.Name {
		case "type":
			roleType = opt.StringValue()
		case "role":
			roleID = opt.RoleValue(s, i.GuildID).ID
		}
	}

	if roleType == "" || roleID == "" {
		return respondEphemeral(s, i, "Missing parameters. Try again without wasting my time.")
	}

	switch roleType {
	case "tasker":
		err := st.SetTaskRole(i.GuildID, roleID)
		if err != nil {
			return respondEphemeral(s, i, fmt.Sprintf("Something broke when saving. Error: `%s`", err.Error()))
		}
	default:
		err := st.SetPunishRole(i.GuildID, roleType, roleID)
		if err != nil {
			return respondEphemeral(s, i, fmt.Sprintf("Something broke when saving. Error: `%s`", err.Error()))
		}
	}

	var response string
	if roleType == "tasker" {
		roleName, err := getRoleNameByID(s, i.GuildID, roleID)
		if err != nil {
			roleName = roleID
		}
		response = fmt.Sprintf("✅ Added **%s** to the list of tasker roles. Update your tasks accordingly.", roleName)
	} else {
		response = fmt.Sprintf("✅ The **%s** role has been updated.", roleType)
	}

	err := respondEphemeral(s, i, response)
	if err != nil {
		return err
	}

	err = logCommand(s, st, i.GuildID, i.ChannelID, i.Member.User.ID, i.Member.User.Username, c.Name())
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
	Register(WithGuildOnly(&SetRolesCommand{}))
}
