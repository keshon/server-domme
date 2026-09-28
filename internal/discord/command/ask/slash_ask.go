package ask

import (
	"fmt"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
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
// conversation both sides agreed to and either party may do it.
const (
	actionAccept = "accept"
	actionDeny   = "deny"
	actionRevoke = "revoke"
	actionClose  = "close"
)

// Status markers.
//
// Nothing is stored: the posted message is the record, and these are what a
// later press reads to recover the state it is acting on.
const (
	markerAccepted = "**accepted**"
	markerDeclined = "**declined**"
	markerRevoked  = "**revoked**"
	markerClosed   = "**closed**"
)

// reasonMarker prefixes the requester's stated reason inside the description.
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

func (c *AskCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionString,
				Name:        "consent_type",
				Description: "What kind of consent are you begging for?",
				Required:    true,
				Choices: []adapter.SlashChoice{
					{Name: "DM Request", Value: "DM"},
					{Name: "Friend Request", Value: "Friend Request"},
					{Name: "Other Reason", Value: "Other Reason"},
				},
			},
			{
				Type:        adapter.OptionUser,
				Name:        "member",
				Description: "Who are you hoping to grovel before?",
				Required:    true,
			},
			{
				Type:        adapter.OptionString,
				Name:        "reason",
				Description: "Be more specific about your request",
				Required:    false,
			},
		},
	}
}

func (c *AskCommand) Run(ctx *adapter.SlashInteractionContext) error {
	consentType := ctx.StringOption("consent_type")
	targetID := ctx.StringOption("member")
	reason := ctx.StringOption("reason")

	askerID := ctx.UserID()
	if targetID == "" || targetID == askerID {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "You can't ask for permission to contact yourself.",
		})
	}

	embed := &adapter.Embed{
		Title:       strings.ToUpper(consentType),
		Description: fmt.Sprintf("<@%s> wants to **%s** <@%s>%s", askerID, consentType, targetID, formatReason(reason)),
		Color:       reply.EmbedColor,
	}

	customPrefix := fmt.Sprintf("ask:%s:%s:%s", askerID, targetID, consentType)

	// Answered rather than followed up: the message id comes back, and the DM
	// below links the request itself rather than the channel it sits in.
	channelID, messageID, err := ctx.AnswerEmbedMessageWithButtons(embed, []adapter.ActionRow{{
		Buttons: []adapter.Button{
			{Label: "✅ Accept", Style: adapter.SecondaryButton, CustomID: customPrefix + ":" + actionAccept},
			{Label: "❌ Deny", Style: adapter.SecondaryButton, CustomID: customPrefix + ":" + actionDeny},
			{Label: "🚫 Revoke", Style: adapter.SecondaryButton, CustomID: customPrefix + ":" + actionRevoke},
		},
	}})
	if err != nil {
		return fmt.Errorf("ask: failed to respond to interaction: %w", err)
	}

	dmUser(ctx.AppLog, ctx.API, targetID, fmt.Sprintf(
		"<@%s> wants to **%s** with you.\nhttps://discord.com/channels/%s/%s/%s",
		askerID, consentType, ctx.GuildID(), channelID, messageID,
	))

	return nil
}

func (c *AskCommand) Component(ctx *adapter.ComponentInteractionContext) error {
	parts := strings.Split(ctx.ComponentID, ":")

	if len(parts) != 5 || parts[0] != "ask" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Something smells off about this button.",
		})
	}

	askerID, targetID, consentType, action := parts[1], parts[2], parts[3], parts[4]
	clickerID := ctx.UserID()

	if clickerID != askerID && clickerID != targetID {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "This ain't your party. Button's not meant for you.",
		})
	}

	desc := ""
	title := ""
	if ctx.MessageEmbed != nil {
		desc = ctx.MessageEmbed.Description
		title = ctx.MessageEmbed.Title
	}
	state := stateOf(desc)
	msgLink := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", ctx.GuildID(), ctx.ChannelID(), ctx.MessageID)

	action = translateLegacyAction(action, state)

	if msg := refusal(action, state, clickerID, askerID, targetID); msg != "" {
		return ctx.RespondEphemeral(&adapter.Embed{Description: msg})
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

	updated := &adapter.Embed{
		Title:       title,
		Description: status + carryReason(desc),
		Color:       reply.EmbedColor,
	}

	// Only an accepted request keeps a button. Every other outcome is terminal:
	// a denial is not something to undo, and a revoked or closed request is
	// restarted by asking again, not by pressing anything here.
	var buttons []adapter.ActionRow
	if action == actionAccept {
		buttons = []adapter.ActionRow{{
			Buttons: []adapter.Button{
				{
					Label:    "🔒 Close",
					Style:    adapter.SecondaryButton,
					CustomID: fmt.Sprintf("ask:%s:%s:%s:%s", askerID, targetID, consentType, actionClose),
				},
			},
		}}
	}

	if err := ctx.ReplaceMessage(adapter.Reply{Embed: updated, Buttons: buttons}); err != nil {
		return fmt.Errorf("ask: failed to update message: %w", err)
	}

	notifyParticipants(ctx.AppLog, ctx.API, action, askerID, targetID, clickerID, consentType, msgLink)

	return nil
}

// translateLegacyAction maps a button posted before Close existed onto the act
// it means today. Those messages carry :revoke on an already-accepted request,
// where revoke meant "end the agreement" — which is now close.
func translateLegacyAction(action string, state askState) string {
	if action == actionRevoke && state == stateActive {
		return actionClose
	}
	return action
}

// refusal reports why a press cannot proceed, or "" when it may.
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

func notifyParticipants(log zerolog.Logger, api adapter.SessionAPI, action, askerID, targetID, clickerID, consentType, link string) {
	switch action {
	case actionAccept:
		dmUser(log, api, askerID,
			fmt.Sprintf("<@%s> accepted your **%s** request.\n%s", targetID, consentType, link))
		dmUser(log, api, targetID,
			fmt.Sprintf("You accepted <@%s>'s **%s** request.\n%s", askerID, consentType, link))

	case actionDeny:
		dmUser(log, api, askerID,
			fmt.Sprintf("<@%s> denied your **%s** request.\n%s", targetID, consentType, link))
		dmUser(log, api, targetID,
			fmt.Sprintf("You denied <@%s>'s **%s** request.\n%s", askerID, consentType, link))

	case actionRevoke:
		dmUser(log, api, askerID,
			fmt.Sprintf("You revoked your **%s** request to <@%s>.\n%s", consentType, targetID, link))
		dmUser(log, api, targetID,
			fmt.Sprintf("<@%s> revoked their **%s** request to you.\n%s", askerID, consentType, link))

	case actionClose:
		other := otherParty(clickerID, askerID, targetID)
		dmUser(log, api, clickerID,
			fmt.Sprintf("You closed the **%s** conversation with <@%s>. Permission ends here — a new request is needed to reopen it.\n%s",
				consentType, other, link))
		dmUser(log, api, other,
			fmt.Sprintf("<@%s> closed the **%s** conversation with you. Permission ends here — a new request is needed to reopen it.\n%s",
				clickerID, consentType, link))
	}
}

// dmUser sends one DM, logging rather than failing. A member who has DMs
// closed is the common case here, not an error worth failing the interaction
// over — the outcome is already recorded on the message by the time this runs.
func dmUser(log zerolog.Logger, api adapter.SessionAPI, userID, content string) {
	if api == nil {
		return
	}
	if err := api.SendDirectMessage(userID, content); err != nil {
		log.Debug().Str("user_id", userID).Err(err).Msg("ask_dm_send_failed")
	}
}
