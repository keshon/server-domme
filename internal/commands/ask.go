package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Disabled:       true,
		Sort:           50,
		Name:           "ask",
		Description:    "Request permission to contact another member",
		Category:       "Etiquette",
		DCSlashHandler: askSlashHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "consent_type",
				Description: "What kind of consent are you begging for?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "DM", Value: "DM"},
					{Name: "Friend Request", Value: "Friend Request"},
					{Name: "Physical Contact", Value: "Physical Contact"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "member",
				Description: "Who are you hoping to grovel before?",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "reason",
				Description: "Why should they even consider it?",
				Required:    false,
			},
		},
		DCComponentHandler: askComponentHandler,
	})
}

func askSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate
	options := i.ApplicationCommandData().Options
	var consentType, reason string
	var member *discordgo.User

	for _, opt := range options {
		switch opt.Name {
		case "consent_type":
			consentType = opt.StringValue()
		case "member":
			member = opt.UserValue(s)
		case "reason":
			reason = opt.StringValue()
		}
	}

	if member == nil || member.ID == i.Member.User.ID {
		respondEphemeral(s, i, "You must pick someone *other* than yourself, sweetheart.")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s request", strings.ToUpper(consentType)),
		Description: fmt.Sprintf("<@%s> wishes to request **%s** from <@%s>%s", i.Member.User.ID, consentType, member.ID, reasonSuffix(reason)),
		Color:       0xB7410E,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "✅ Accept", Style: discordgo.SuccessButton, CustomID: "ask_accept_" + i.ID},
					discordgo.Button{Label: "❌ Deny", Style: discordgo.DangerButton, CustomID: "ask_deny_" + i.ID},
					discordgo.Button{Label: "🚫 Revoke", Style: discordgo.SecondaryButton, CustomID: "ask_revoke_" + i.ID},
				}},
			},
		},
	})

	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("You've been asked for **%s** by <@%s>. Please check the channel to respond.", consentType, i.Member.User.ID),
	})
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return fmt.Sprintf("\n\n**Reason:** %s", reason)
}

func askComponentHandler(ctx *ComponentContext) {
	s, i := ctx.Session, ctx.InteractionCreate
	customID := i.MessageComponentData().CustomID

	action := ""
	if strings.HasPrefix(customID, "ask_accept_") {
		action = "accepted"
	} else if strings.HasPrefix(customID, "ask_deny_") {
		action = "denied"
	} else if strings.HasPrefix(customID, "ask_revoke_") {
		action = "revoked"
	} else {
		respondEphemeral(s, i, "Unknown button pressed. What did you *think* that would do?")
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    fmt.Sprintf("Request has been **%s** by <@%s>.", action, i.Member.User.ID),
			Components: []discordgo.MessageComponent{},
		},
	})

	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("You have **%s** the request.", action),
	})
}
