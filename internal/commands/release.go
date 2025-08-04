package commands

import (
	"fmt"
	"log"
	"slices"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           40,
		Name:           "release",
		Category:       "🎭 Roleplay",
		Description:    "Remove the brat role and grant reprieve",
		DCSlashHandler: releaseSlashHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "target",
				Description: "The brat to be released",
				Required:    true,
			},
		},
	})
}

func releaseSlashHandler(ctx *SlashContext) {
	if !RequireGuild(ctx) {
		return
	}

	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options

	punisherRoleID, err := storage.GetPunishRole(i.GuildID, "punisher")
	if err != nil || punisherRoleID == "" {
		respondEphemeral(s, i, "No 'punisher' role set? Assign the role with `/set-role punisher` that can use this command.")
		return
	}

	assignedRoleID, err := storage.GetPunishRole(i.GuildID, "assigned")
	if err != nil || assignedRoleID == "" {
		respondEphemeral(s, i, "The 'assigned' role isn’t even set up. Assign the role with `/set-role assigned` that can be released from.")
		return
	}

	if !slices.Contains(i.Member.Roles, punisherRoleID) {
		respondEphemeral(s, i, "YNo, no, no. You don’t *get* to undo what the real dommes do. Back to your corner.")
		return
	}

	var targetUserID string
	for _, opt := range options {
		if opt.Name == "target" && opt.Type == discordgo.ApplicationCommandOptionUser {
			targetUserID = opt.Value.(string)
			break
		}
	}

	if targetUserID == "" {
		respondEphemeral(s, i, "Release who, darling? The void?")
		return
	}

	err = s.GuildMemberRoleRemove(i.GuildID, targetUserID, assignedRoleID)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Tried to undo their sentence, but the chains are tight: ```%v```", err))
		return
	}

	respond(s, i, fmt.Sprintf("🔓 <@%s> has been released. Let's see if they behave. Doubt it.", targetUserID))

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "release")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
