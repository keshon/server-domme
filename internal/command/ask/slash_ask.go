package ask

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/rs/zerolog"
)

type AskCommand struct{}

func (c *AskCommand) Name() string        { return "ask" }
func (c *AskCommand) Description() string { return "Ask for permission to contact another member" }
func (c *AskCommand) Group() string       { return "ask" }
func (c *AskCommand) Category() string    { return "🎭 Roleplay" }
func (c *AskCommand) UserPermissions() []int64 {
	return []int64{}
}

// Button actions.
//
// revoke and close are deliberately separate acts: revoke takes back a request
// nobody has answered yet and belongs to the asker alone, while close ends a
// conversation both sides agreed to and either party may do it. Serving both
// from one action is what forced the old handler to guess which was meant by
// reading the embed, and is why a denied request offered to "revoke an
// agreement" that never existed.
const (
	actionAccept = "accept"
	actionDeny   = "deny"
	actionRevoke = "revoke"
	actionClose  = "close"
)

// Status markers.
//
// Nothing is stored: the posted message is the record, and these are what a
// later press reads to recover the state it is acting on. A status string and
// its marker therefore have to change together — reword one without the other
// and every button already sitting in a channel starts misreading its state.
const (
	markerAccepted = "**accepted**"
	markerDeclined = "**declined**"
	markerRevoked  = "**revoked**"
	markerClosed   = "**closed**"
)

// reasonMarker prefixes the requester's stated reason inside the description.
// The description is the only place it lives, so every transition has to carry
// it forward, and the marker has to survive that unchanged — otherwise the
// transition after it can no longer find the reason.
const reasonMarker = "Reason:"

type askState int

const (
	statePending askState = iota
	stateActive
	stateDeclined
	stateFinished
)

func stateOf(desc string) askState {
	switch {
	case strings.Contains(desc, markerAccepted):
		return stateActive
	case strings.Contains(desc, markerDeclined):
		return stateDeclined
	case strings.Contains(desc, markerRevoked), strings.Contains(desc, markerClosed):
		return stateFinished
	default:
		return statePending
	}
}

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
	context, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event

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
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "You can't ask for permission to contact yourself.",
		})
		return nil
	}

	embed := &discordgo.MessageEmbed{
		Title:       strings.ToUpper(consentType),
		Description: fmt.Sprintf("<@%s> wants to **%s** <@%s>%s", askerID, consentType, targetUser.ID, formatReason(reason)),
		Color:       reply.EmbedColor,
	}

	customPrefix := fmt.Sprintf("ask:%s:%s:%s", askerID, targetUser.ID, consentType)

	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "✅ Accept", Style: discordgo.SecondaryButton, CustomID: customPrefix + ":" + actionAccept},
					discordgo.Button{Label: "❌ Deny", Style: discordgo.SecondaryButton, CustomID: customPrefix + ":" + actionDeny},
					discordgo.Button{Label: "🚫 Revoke", Style: discordgo.SecondaryButton, CustomID: customPrefix + ":" + actionRevoke},
				}},
			},
		},
	}); err != nil {
		return fmt.Errorf("ask: failed to respond to interaction: %w", err)
	}

	dm := fmt.Sprintf(
		"<@%s> wants to **%s** with you.\nhttps://discord.com/channels/%s/%s/%s",
		askerID, consentType, event.GuildID, event.ChannelID, event.ID,
	)

	dmUser(context.AppLog, session, targetUser.ID, dm)

	return nil
}

func (c *AskCommand) Component(ctx *cmdadapter.ComponentInteractionContext) error {
	session, event := ctx.Session, ctx.Event
	customID := event.MessageComponentData().CustomID
	parts := strings.Split(customID, ":")

	if len(parts) != 5 || parts[0] != "ask" {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Something smells off about this button.",
		})
		return nil
	}

	askerID, targetID, consentType, action := parts[1], parts[2], parts[3], parts[4]
	clickerID := event.Member.User.ID

	if clickerID != askerID && clickerID != targetID {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "This ain't your party. Button's not meant for you.",
		})
		return nil
	}

	embed := event.Message.Embeds[0]
	desc := embed.Description
	state := stateOf(desc)
	msgLink := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", event.GuildID, event.ChannelID, event.Message.ID)

	action = translateLegacyAction(action, state)

	if msg := refusal(action, state, clickerID, askerID, targetID); msg != "" {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{Description: msg})
		return nil
	}

	var status string
	switch action {
	case actionAccept:
		status = fmt.Sprintf("<@%s> %s <@%s>'s **%s** request.", targetID, markerAccepted, askerID, consentType)
	case actionDeny:
		status = fmt.Sprintf("<@%s> %s <@%s>'s **%s** request.", targetID, markerDeclined, askerID, consentType)
	case actionRevoke:
		status = fmt.Sprintf("<@%s> %s their **%s** request to <@%s>.", askerID, markerRevoked, consentType, targetID)
	case actionClose:
		status = fmt.Sprintf("<@%s> %s the **%s** conversation with <@%s>.",
			clickerID, markerClosed, consentType, otherParty(clickerID, askerID, targetID))
	}

	updated := &discordgo.MessageEmbed{
		Title:       embed.Title,
		Description: status + carryReason(desc),
		Color:       reply.EmbedColor,
	}

	// Only an accepted request keeps a button. Every other outcome is terminal:
	// a denial is not something to undo, and a revoked or closed request is
	// restarted by asking again, not by pressing anything here.
	var components []discordgo.MessageComponent
	if action == actionAccept {
		components = []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "🔒 Close",
						Style:    discordgo.SecondaryButton,
						CustomID: fmt.Sprintf("ask:%s:%s:%s:%s", askerID, targetID, consentType, actionClose),
					},
				},
			},
		}
	}

	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{updated},
			Components: components,
		},
	}); err != nil {
		return fmt.Errorf("ask: failed to update message: %w", err)
	}

	notifyParticipants(ctx.AppLog, session, action, askerID, targetID, clickerID, consentType, msgLink)

	return nil
}

// translateLegacyAction maps a button posted before Close existed onto the act
// it means today.
//
// Those messages carry :revoke on an already-accepted request, where revoke
// meant "end the agreement" — which is now close. They are still sitting in
// channels and their ids come back whenever someone presses one, so the id is
// translated rather than repointed. Do not "simplify" this away by reusing
// :revoke for closing: the two actions have different rules about who may press
// them, and merging them is what produced the original confusion.
func translateLegacyAction(action string, state askState) string {
	if action == actionRevoke && state == stateActive {
		return actionClose
	}
	return action
}

// refusal reports why a press cannot proceed, or "" when it may. Both halves
// matter: the wrong person pressing, and the right person pressing a button
// belonging to a state the request has already left.
func refusal(action string, state askState, clickerID, askerID, targetID string) string {
	switch action {
	case actionAccept, actionDeny:
		if clickerID != targetID {
			return "Only the recipient of this request can respond. If you're the sender, you can still revoke it before they decide."
		}
		if state != statePending {
			return "That's already been answered."
		}
	case actionRevoke:
		if clickerID != askerID {
			return "Only the requester can withdraw this offer before it's answered."
		}
		if state != statePending {
			return "Too late to withdraw — that's already been answered."
		}
	case actionClose:
		if state != stateActive {
			return "There's no open conversation here to close."
		}
	default:
		return "Unknown action. Not touching that."
	}
	return ""
}

// otherParty returns whichever of the two participants did not press.
func otherParty(clickerID, askerID, targetID string) string {
	if clickerID == targetID {
		return askerID
	}
	return targetID
}

func formatReason(r string) string {
	if r == "" {
		return ""
	}
	return "\n\n" + reasonMarker + "\n`" + r + "`"
}

// carryReason returns the reason block from a description, ready to append to
// the next status. Empty when the request carried no reason.
func carryReason(desc string) string {
	idx := strings.Index(desc, reasonMarker)
	if idx == -1 {
		return ""
	}
	rest := strings.TrimSpace(desc[idx+len(reasonMarker):])
	if rest == "" {
		return ""
	}
	return "\n\n" + reasonMarker + "\n" + rest
}

func notifyParticipants(log zerolog.Logger, session *discordgo.Session, action, askerID, targetID, clickerID, consentType, link string) {
	switch action {
	case actionAccept:
		dmUser(log, session, askerID,
			fmt.Sprintf("<@%s> accepted your **%s** request.\n%s", targetID, consentType, link))
		dmUser(log, session, targetID,
			fmt.Sprintf("You accepted <@%s>'s **%s** request.\n%s", askerID, consentType, link))

	case actionDeny:
		dmUser(log, session, askerID,
			fmt.Sprintf("<@%s> denied your **%s** request.\n%s", targetID, consentType, link))
		dmUser(log, session, targetID,
			fmt.Sprintf("You denied <@%s>'s **%s** request.\n%s", askerID, consentType, link))

	case actionRevoke:
		dmUser(log, session, askerID,
			fmt.Sprintf("You revoked your **%s** request to <@%s>.\n%s", consentType, targetID, link))
		dmUser(log, session, targetID,
			fmt.Sprintf("<@%s> revoked their **%s** request to you.\n%s", askerID, consentType, link))

	case actionClose:
		other := otherParty(clickerID, askerID, targetID)
		dmUser(log, session, clickerID,
			fmt.Sprintf("You closed the **%s** conversation with <@%s>. Permission ends here — a new request is needed to reopen it.\n%s",
				consentType, other, link))
		dmUser(log, session, other,
			fmt.Sprintf("<@%s> closed the **%s** conversation with you. Permission ends here — a new request is needed to reopen it.\n%s",
				clickerID, consentType, link))
	}
}

// dmUser opens the user's DM channel and sends one message, logging rather than
// failing on either step. A member who has DMs closed is the common case here,
// not an error worth failing the interaction over — the outcome is already
// recorded on the message by the time this runs.
func dmUser(log zerolog.Logger, s *discordgo.Session, userID, content string) {
	ch, err := s.UserChannelCreate(userID)
	if err != nil {
		log.Debug().Str("user_id", userID).Err(err).Msg("ask_dm_channel_failed")
		return
	}
	if _, err := s.ChannelMessageSend(ch.ID, content); err != nil {
		log.Debug().Str("user_id", userID).Err(err).Msg("ask_dm_send_failed")
	}
}
