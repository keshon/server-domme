package commands

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           10,
		Name:           "ask",
		Description:    "Request permission to contact another member.",
		Category:       "🎭 Roleplay",
		DCSlashHandler: askSlashHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "consent_type",
				Description: "What kind of consent are you begging for?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "DM Request", Value: "DM"},
					{Name: "Friend Request", Value: "Friend Request"},
					{Name: "Other Reason", Value: "Other Reason"},
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
				Description: "Be more specific about your request",
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
	var targetUser *discordgo.User

	for _, opt := range options {
		switch opt.Name {
		case "consent_type":
			consentType = opt.StringValue()
		case "member":
			targetUser = opt.UserValue(s)
		case "reason":
			reason = opt.StringValue()
		}
	}

	askerID := i.Member.User.ID
	if targetUser == nil || targetUser.ID == askerID {
		respondEphemeral(s, i, "You must pick someone *other* than yourself, sweetheart.")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       strings.ToUpper(consentType),
		Description: fmt.Sprintf("<@%s> wants to **%s** <@%s>%s", askerID, consentType, targetUser.ID, reasonSuffix(reason)),
		Color:       embedColor,
	}

	customPrefix := fmt.Sprintf("ask:%s:%s:%s", askerID, targetUser.ID, consentType)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "✅ Accept", Style: discordgo.SecondaryButton, CustomID: customPrefix + ":accept"},
					discordgo.Button{Label: "❌ Deny", Style: discordgo.SecondaryButton, CustomID: customPrefix + ":deny"},
					discordgo.Button{Label: "🚫 Revoke", Style: discordgo.SecondaryButton, CustomID: customPrefix + ":revoke"},
				}},
			},
		},
	})
}

func askComponentHandler(ctx *ComponentContext) {
	s, i := ctx.Session, ctx.InteractionCreate
	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, ":")
	if len(parts) != 5 || parts[0] != "ask" {
		respondEphemeral(s, i, "That button seems... suspicious. Try clicking something real next time.")
		return
	}

	askerID, targetID, consentType, action := parts[1], parts[2], parts[3], parts[4]
	clickerID := i.Member.User.ID

	switch action {
	case "accept", "deny":
		if clickerID != targetID {
			respondEphemeral(s, i, "Oh no no no. Only the *chosen one* can respond to this request.")
			return
		}
	case "revoke":
		if clickerID != askerID {
			respondEphemeral(s, i, "Only the beggar may revoke their plea.")
			return
		}
	default:
		respondEphemeral(s, i, "Unknown action. What sort of mischief are you up to?")
		return
	}

	originalEmbed := i.Message.Embeds[0]
	reason := extractReason(originalEmbed.Description)
	msgLink := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", i.GuildID, i.ChannelID, i.Message.ID)

	var status string
	switch action {
	case "accept":
		status = fmt.Sprintf("<@%s> **accepted** <@%s>'s **%s** request.", targetID, askerID, consentType)
	case "deny":
		status = fmt.Sprintf("<@%s> **declined** <@%s>'s **%s** request.", targetID, askerID, consentType)
	case "revoke":
		status = fmt.Sprintf("<@%s> **revoked** their **%s** request to <@%s>. How tragic.", askerID, consentType, targetID)
	}

	newEmbed := &discordgo.MessageEmbed{
		Title:       originalEmbed.Title,
		Description: fmt.Sprintf("%s\n\n%s", status, reason),
		Color:       originalEmbed.Color,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{newEmbed},
			Components: []discordgo.MessageComponent{},
		},
	})

	notifyUsersWithLink(s, action, askerID, targetID, consentType, msgLink)
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return fmt.Sprintf("\n\nReason:\n`%s`", reason)
}

func extractReason(desc string) string {
	idx := strings.Index(desc, "Reason:")
	if idx == -1 {
		return ""
	}

	reason := desc[idx+len("Reason:"):]
	return fmt.Sprintf("The request reason was:\n`%s`", strings.TrimSpace(reason))
}

func notifyUsersWithLink(s *discordgo.Session, action, askerID, targetID, consentType, msgLink string) {
	var msg string
	switch action {
	case "accept":
		msg = fmt.Sprintf("Your request to <@%s> for **%s** was *accepted*.\n%s", targetID, consentType, msgLink)
		s.ChannelMessageSendComplex(dmChannel(s, askerID), &discordgo.MessageSend{Content: msg})

		msg = fmt.Sprintf("You accepted <@%s>'s **%s** request.\n%s", askerID, consentType, msgLink)
		s.ChannelMessageSendComplex(dmChannel(s, targetID), &discordgo.MessageSend{Content: msg})

	case "deny":
		msg = fmt.Sprintf("Your request to <@%s> for **%s** was *denied*.\n%s", targetID, consentType, msgLink)
		s.ChannelMessageSendComplex(dmChannel(s, askerID), &discordgo.MessageSend{Content: msg})

		msg = fmt.Sprintf("You denied <@%s>'s **%s** request.\n%s", askerID, consentType, msgLink)
		s.ChannelMessageSendComplex(dmChannel(s, targetID), &discordgo.MessageSend{Content: msg})

	case "revoke":
		msg = fmt.Sprintf("You revoked your **%s** request to <@%s>.\n%s", consentType, targetID, msgLink)
		s.ChannelMessageSendComplex(dmChannel(s, askerID), &discordgo.MessageSend{Content: msg})

		msg = fmt.Sprintf("<@%s> revoked their **%s** request to you.\n%s", askerID, consentType, msgLink)
		s.ChannelMessageSendComplex(dmChannel(s, targetID), &discordgo.MessageSend{Content: msg})
	}
}

func dmChannel(s *discordgo.Session, userID string) string {
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return ""
	}
	return channel.ID
}
