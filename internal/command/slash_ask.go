// /internal/command/ask.go
package command

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type AskCommand struct{}

func (c *AskCommand) Name() string        { return "ask" }
func (c *AskCommand) Description() string { return "Request permission to contact another member" }
func (c *AskCommand) Category() string    { return "🎭 Roleplay" }
func (c *AskCommand) Aliases() []string   { return nil }
func (c *AskCommand) RequireAdmin() bool  { return false }
func (c *AskCommand) RequireDev() bool    { return false }

func (c *AskCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
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
	}
}

func (c *AskCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context")
	}
	session := slash.Session
	event := slash.Event
	options := event.ApplicationCommandData().Options

	var consentType, reason string
	var targetUser *discordgo.User

	for _, opt := range options {
		switch opt.Name {
		case "consent_type":
			consentType = opt.StringValue()
		case "member":
			targetUser = opt.UserValue(session)
		case "reason":
			reason = opt.StringValue()
		}
	}

	askerID := event.Member.User.ID
	if targetUser == nil || targetUser.ID == askerID {
		respondEphemeral(session, event, "Pick someone other than yourself, darling.")
		return nil
	}

	embed := &discordgo.MessageEmbed{
		Title:       strings.ToUpper(consentType),
		Description: fmt.Sprintf("<@%s> wants to **%s** <@%s>%s", askerID, consentType, targetUser.ID, formatReason(reason)),
		Color:       embedColor,
	}

	customPrefix := fmt.Sprintf("ask:%s:%s:%s", askerID, targetUser.ID, consentType)

	session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
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

	dm := fmt.Sprintf(
		"<@%s> wants to **%s** with you.\nRequest: https://discord.com/channels/%s/%s/%s",
		askerID, consentType, event.GuildID, event.ChannelID, event.ID,
	)
	session.ChannelMessageSend(dmChannel(session, targetUser.ID), dm)

	logCommand(session, slash.Storage, event.GuildID, event.ChannelID, askerID, event.Member.User.Username, "ask")
	return nil
}

func (c *AskCommand) Component(ctx *ComponentContext) error {
	session, event := ctx.Session, ctx.Event
	customID := event.MessageComponentData().CustomID
	parts := strings.Split(customID, ":")
	if len(parts) != 5 || parts[0] != "ask" {
		respondEphemeral(session, event, "Something smells off about this button.")
		return nil
	}

	askerID, targetID, consentType, action := parts[1], parts[2], parts[3], parts[4]
	clickerID := event.Member.User.ID

	switch action {
	case "accept", "deny":
		if clickerID != targetID {
			respondEphemeral(session, event, "Only the target can do that.")
			return nil
		}
	case "revoke":
		if clickerID != askerID {
			respondEphemeral(session, event, "Only the one who begged can revoke it.")
			return nil
		}
	default:
		respondEphemeral(session, event, "Unknown action. Not touching that.")
		return nil
	}

	desc := event.Message.Embeds[0].Description
	reason := extractReason(desc)
	msgLink := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", event.GuildID, event.ChannelID, event.Message.ID)

	var status string
	switch action {
	case "accept":
		status = fmt.Sprintf("<@%s> **accepted** <@%s>'s **%s** request.", targetID, askerID, consentType)
	case "deny":
		status = fmt.Sprintf("<@%s> **declined** <@%s>'s **%s** request.", targetID, askerID, consentType)
	case "revoke":
		status = fmt.Sprintf("<@%s> **revoked** their **%s** request to <@%s>.", askerID, consentType, targetID)
	}

	updated := &discordgo.MessageEmbed{
		Title:       event.Message.Embeds[0].Title,
		Description: fmt.Sprintf("%s\n\n%s", status, reason),
		Color:       embedColor,
	}

	session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{updated},
			Components: []discordgo.MessageComponent{},
		},
	})

	notify(session, action, askerID, targetID, consentType, msgLink, reason)
	return nil
}

func formatReason(r string) string {
	if r == "" {
		return ""
	}
	return fmt.Sprintf("\n\nReason:\n`%s`", r)
}

func extractReason(desc string) string {
	idx := strings.Index(desc, "Reason:")
	if idx == -1 {
		return ""
	}
	return fmt.Sprintf("Reason was:\n`%s`", strings.TrimSpace(desc[idx+len("Reason:"):]))
}

func notify(session *discordgo.Session, action, askerID, targetID, consentType, link, reason string) {
	var msg string
	switch action {
	case "accept":
		msg = fmt.Sprintf("Your request to <@%s> for **%s** was accepted.\n%s", targetID, consentType, link)
		session.ChannelMessageSend(dmChannel(session, askerID), msg)
		session.ChannelMessageSend(dmChannel(session, targetID), fmt.Sprintf("You accepted <@%s>'s request.\n%s", askerID, link))

	case "deny":
		msg = fmt.Sprintf("Your request to <@%s> for **%s** was denied.\n%s", targetID, consentType, link)
		session.ChannelMessageSend(dmChannel(session, askerID), msg)
		session.ChannelMessageSend(dmChannel(session, targetID), fmt.Sprintf("You declined <@%s>'s request.\n%s", askerID, link))

	case "revoke":
		msg = fmt.Sprintf("You revoked your **%s** request to <@%s>.\n%s", consentType, targetID, link)
		session.ChannelMessageSend(dmChannel(session, askerID), msg)
		session.ChannelMessageSend(dmChannel(session, targetID), fmt.Sprintf("<@%s> revoked their request to you.\n%s", askerID, link))
	}
}

func dmChannel(s *discordgo.Session, userID string) string {
	ch, err := s.UserChannelCreate(userID)
	if err != nil {
		return ""
	}
	return ch.ID
}

func init() {
	Register(WithGuildOnly(&AskCommand{}))
}
