// /internal/command/ask.go
package command

import (
	"fmt"
	"log"
	"server-domme/internal/core"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type AskCommand struct{}

func (c *AskCommand) Name() string        { return "ask" }
func (c *AskCommand) Description() string { return "Request permission to contact another member" }
func (c *AskCommand) Aliases() []string   { return []string{} }
func (c *AskCommand) Group() string       { return "ask" }
func (c *AskCommand) Category() string    { return "🎭 Roleplay" }
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
	slash, ok := ctx.(*core.SlashContext)
	if !ok {
		return fmt.Errorf("wrong context")
	}

	session := slash.Session
	event := slash.Event
	options := event.ApplicationCommandData().Options
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

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
		core.RespondEphemeral(session, event, "Pick someone other than yourself, darling.")
		return nil
	}

	embed := &discordgo.MessageEmbed{
		Title:       strings.ToUpper(consentType),
		Description: fmt.Sprintf("<@%s> wants to **%s** <@%s>%s", askerID, consentType, targetUser.ID, formatReason(reason)),
		Color:       core.EmbedColor,
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
		"<@%s> wants to **%s** with you.\nhttps://discord.com/channels/%s/%s/%s",
		askerID, consentType, event.GuildID, event.ChannelID, event.ID,
	)
	session.ChannelMessageSend(dmChannel(session, targetUser.ID), dm)

	err := core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func (c *AskCommand) Component(ctx *core.ComponentContext) error {
	session, event := ctx.Session, ctx.Event
	customID := event.MessageComponentData().CustomID
	parts := strings.Split(customID, ":")

	if len(parts) != 5 || parts[0] != "ask" {
		core.RespondEphemeral(session, event, "Something smells off about this button.")
		return nil
	}

	askerID, targetID, consentType, action := parts[1], parts[2], parts[3], parts[4]
	clickerID := event.Member.User.ID

	if clickerID != askerID && clickerID != targetID {
		core.RespondEphemeral(session, event, "This ain't your party. Button's not meant for you.")
		return nil
	}

	embed := event.Message.Embeds[0]
	desc := embed.Description
	reason := extractReason(desc)
	msgLink := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", event.GuildID, event.ChannelID, event.Message.ID)

	alreadyAnswered := strings.Contains(desc, "**accepted**") || strings.Contains(desc, "**declined**")

	if action == "revoke" {
		if alreadyAnswered {
			if clickerID != targetID {
				core.RespondEphemeral(session, event, "Too late to chicken out. The ball's not in your court anymore. This button is for the other side to decide at this point.")
				return nil
			}
		} else {
			if clickerID != askerID {
				core.RespondEphemeral(session, event, "At this point only the one who begged can revoke this. You can use this button once you accept the request (or just ignore it).")
				return nil
			}
		}
	}

	if action == "accept" || action == "deny" {
		if clickerID != targetID {
			core.RespondEphemeral(session, event, "Oh no no no. Only the *chosen one* can respond to this request. But if you changed your mind, use the revoke button until it's too late.")
			return nil
		}
	}

	var status string
	switch action {
	case "accept":
		status = fmt.Sprintf("<@%s> **accepted** <@%s>'s **%s** request.", targetID, askerID, consentType)
	case "deny":
		status = fmt.Sprintf("<@%s> **declined** <@%s>'s **%s** request.", targetID, askerID, consentType)
	case "revoke":
		if alreadyAnswered {
			status = fmt.Sprintf("<@%s> **revoked** their agreement with <@%s>.", targetID, askerID)
		} else {
			status = fmt.Sprintf("<@%s> **revoked** their **%s** request to <@%s>.", askerID, consentType, targetID)
		}
	default:
		core.RespondEphemeral(session, event, "Unknown action. Not touching that.")
		return nil
	}

	updated := &discordgo.MessageEmbed{
		Title:       embed.Title,
		Description: fmt.Sprintf("%s\n\n%s", status, reason),
		Color:       core.EmbedColor,
	}

	var components []discordgo.MessageComponent

	if action == "accept" || action == "deny" {
		components = []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "🚫 Revoke",
						Style:    discordgo.SecondaryButton,
						CustomID: fmt.Sprintf("ask:%s:%s:%s:revoke", askerID, targetID, consentType),
					},
				},
			},
		}
	}

	session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{updated},
			Components: components,
		},
	})

	notifyParticipants(session, action, askerID, targetID, clickerID, consentType, msgLink)

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

func notifyParticipants(session *discordgo.Session, action, askerID, targetID, clickerID, consentType, link string) {
	switch action {
	case "accept":
		session.ChannelMessageSend(dmChannel(session, askerID),
			fmt.Sprintf("<@%s> accepted your **%s** request.\n%s", targetID, consentType, link))
		session.ChannelMessageSend(dmChannel(session, targetID),
			fmt.Sprintf("You accepted <@%s>'s **%s** request.\n%s", askerID, consentType, link))

	case "deny":
		session.ChannelMessageSend(dmChannel(session, askerID),
			fmt.Sprintf("<@%s> denied your **%s** request.\n%s", targetID, consentType, link))
		session.ChannelMessageSend(dmChannel(session, targetID),
			fmt.Sprintf("You denied <@%s>'s **%s** request.\n%s", askerID, consentType, link))

	case "revoke":
		if clickerID == askerID {
			session.ChannelMessageSend(dmChannel(session, askerID),
				fmt.Sprintf("You revoked your **%s** request to <@%s>.\n%s", consentType, targetID, link))
			session.ChannelMessageSend(dmChannel(session, targetID),
				fmt.Sprintf("<@%s> revoked their **%s** request to you.\n%s", askerID, consentType, link))
		} else {
			session.ChannelMessageSend(dmChannel(session, askerID),
				fmt.Sprintf("<@%s> revoked their agreement with you.\n%s", targetID, link))
			session.ChannelMessageSend(dmChannel(session, targetID),
				fmt.Sprintf("You revoked your agreement with <@%s>.\n%s", askerID, link))
		}
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
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&AskCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
