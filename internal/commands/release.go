package commands

import (
	"fmt"
	"slices"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           201,
		Name:           "release",
		Description:    "Release a brat (removes the assigned role)",
		Category:       "Assign brat role",
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
	s, i, storage := ctx.Session, ctx.Interaction, ctx.Storage
	options := i.ApplicationCommandData().Options

	punisherRoleID, err := storage.GetRoleForGuild(i.GuildID, "punisher")
	if err != nil || punisherRoleID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No 'punisher' role set? Someone’s slacking in their duties.",
				Flags:   1 << 6,
			},
		})
		return
	}

	assignedRoleID, err := storage.GetRoleForGuild(i.GuildID, "assigned")
	if err != nil || assignedRoleID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "The 'assigned' role isn’t even set up. Release? From *what*, exactly?",
				Flags:   1 << 6,
			},
		})
		return
	}

	if !slices.Contains(i.Member.Roles, punisherRoleID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No, no, no. You don’t *get* to undo what the real dommes do. Back to your corner.",
				Flags:   1 << 6,
			},
		})
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
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Release who, darling? The void?",
				Flags:   1 << 6,
			},
		})
		return
	}

	err = s.GuildMemberRoleRemove(i.GuildID, targetUserID, assignedRoleID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Tried to undo their sentence, but the chains are tight: ```%v```", err),
				Flags:   1 << 6,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🔓 <@%s> has been released. Let's see if they behave. Doubt it.", targetUserID),
		},
	})
}
