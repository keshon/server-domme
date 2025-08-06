package command

import (
	"fmt"
	"slices"

	"github.com/bwmarrin/discordgo"
)

type ReleaseCommand struct{}

func (c *ReleaseCommand) Name() string        { return "release" }
func (c *ReleaseCommand) Description() string { return "Remove the brat role" }
func (c *ReleaseCommand) Aliases() []string   { return []string{} }

func (c *ReleaseCommand) Group() string    { return "punish" }
func (c *ReleaseCommand) Category() string { return "🎭 Roleplay" }

func (c *ReleaseCommand) RequireAdmin() bool { return false }
func (c *ReleaseCommand) RequireDev() bool   { return false }

func (c *ReleaseCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "target",
				Description: "The brat to be released",
				Required:    true,
			},
		},
	}
}

func (c *ReleaseCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	punisherRoleID, _ := storage.GetPunishRole(event.GuildID, "punisher")
	assignedRoleID, _ := storage.GetPunishRole(event.GuildID, "assigned")

	if punisherRoleID == "" || assignedRoleID == "" {
		respondEphemeral(session, event, "Roles not configured properly. Run `/set-role` first.")
		return nil
	}

	if !slices.Contains(event.Member.Roles, punisherRoleID) {
		respondEphemeral(session, event, "No, no, no. You don’t *get* to undo what the real dommes do. Back to your corner.")
		return nil
	}

	var targetID string
	for _, opt := range event.ApplicationCommandData().Options {
		if opt.Name == "target" {
			targetID = opt.Value.(string)
		}
	}

	if targetID == "" {
		respondEphemeral(session, event, "Release who, darling? The void?")
		return nil
	}

	err := session.GuildMemberRoleRemove(event.GuildID, targetID, assignedRoleID)
	if err != nil {
		respondEphemeral(session, event, fmt.Sprintf("Tried to undo their sentence, but the chains are tight: ```%v```", err))
		return nil
	}

	respond(session, event, fmt.Sprintf("🔓 <@%s> has been released. Let's see if they behave. Doubt it.", targetID))

	logCommand(session, slash.Storage, event.GuildID, event.ChannelID, event.Member.User.ID, event.Member.User.Username, "release")
	return nil
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&ReleaseCommand{},
			),
		),
	)
}
