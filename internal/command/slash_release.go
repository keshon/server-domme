package command

import (
	"fmt"
	"slices"

	"github.com/bwmarrin/discordgo"
)

type ReleaseCommand struct{}

func (c *ReleaseCommand) Name() string        { return "release" }
func (c *ReleaseCommand) Description() string { return "Remove the brat role and grant reprieve" }
func (c *ReleaseCommand) Category() string    { return "🎭 Roleplay" }
func (c *ReleaseCommand) Aliases() []string   { return []string{} }
func (c *ReleaseCommand) RequireAdmin() bool  { return false }
func (c *ReleaseCommand) RequireDev() bool    { return false }

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
	s, i, storage := slash.Session, slash.Event, slash.Storage

	punisherRoleID, _ := storage.GetPunishRole(i.GuildID, "punisher")
	assignedRoleID, _ := storage.GetPunishRole(i.GuildID, "assigned")

	if punisherRoleID == "" || assignedRoleID == "" {
		respondEphemeral(s, i, "Roles not configured properly. Run `/set-role` first.")
		return nil
	}

	if !slices.Contains(i.Member.Roles, punisherRoleID) {
		respondEphemeral(s, i, "No, no, no. You don’t *get* to undo what the real dommes do. Back to your corner.")
		return nil
	}

	var targetID string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "target" {
			targetID = opt.Value.(string)
		}
	}

	if targetID == "" {
		respondEphemeral(s, i, "Release who, darling? The void?")
		return nil
	}

	err := s.GuildMemberRoleRemove(i.GuildID, targetID, assignedRoleID)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Tried to undo their sentence, but the chains are tight: ```%v```", err))
		return nil
	}

	respond(s, i, fmt.Sprintf("🔓 <@%s> has been released. Let's see if they behave. Doubt it.", targetID))

	logCommand(s, slash.Storage, i.GuildID, i.ChannelID, i.Member.User.ID, i.Member.User.Username, "release")
	return nil
}

func init() {
	Register(&ReleaseCommand{})
}
