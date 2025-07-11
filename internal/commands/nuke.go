package commands

import (
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           999,
		Name:           "nuke",
		Description:    "Delete all messages in this channel",
		Category:       "Moderation",
		DCSlashHandler: nukeSlashHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "confirm",
				Description: "Type 'yes' to confirm the action",
				Required:    true,
			},
		},
	})
}

func nukeSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate
	options := i.ApplicationCommandData().Options
	channelID := i.ChannelID

	if !isAdmin(s, i.GuildID, i.Member) {
		respondEphemeral(s, i, "You lack the authority. Go fetch someone with actual power.")
		return
	}

	if !checkBotPermissions(s, channelID) {
		respondEphemeral(s, i, "I can't delete messages here. Try giving me actual permission next time.")
		return
	}

	var confirm string
	for _, opt := range options {
		if opt.Name == "confirm" {
			confirm = opt.StringValue()
			break
		}
	}

	if strings.ToLower(confirm) != "yes" {
		respondEphemeral(s, i, "You must type 'yes' to confirm. Consent is everything, darling.")
		return
	}

	activeDeletionsMu.Lock()
	if _, exists := activeDeletions[channelID]; exists {
		activeDeletionsMu.Unlock()
		respondEphemeral(s, i, "There’s already a deletion happening here. Patience, dear.")
		return
	}

	stopChan := make(chan struct{})
	activeDeletions[channelID] = stopChan
	activeDeletionsMu.Unlock()

	respond(s, i, "Starting to delete... Goodbye, messages.")

	go func() {
		deleteMessages(s, channelID, nil, nil, stopChan)

		activeDeletionsMu.Lock()
		delete(activeDeletions, channelID)
		activeDeletionsMu.Unlock()
	}()
}

func deleteMessages(s *discordgo.Session, channelID string, startTime, endTime *time.Time, stopChan <-chan struct{}) {
	var lastID string

	for {
		select {
		case <-stopChan:
			return
		default:
		}

		msgs, err := s.ChannelMessages(channelID, 100, lastID, "", "")
		if err != nil || len(msgs) == 0 {
			break
		}

		for _, msg := range msgs {
			select {
			case <-stopChan:
				return
			default:
			}

			if startTime != nil && msg.Timestamp.Before(*startTime) {
				continue
			}
			if endTime != nil && msg.Timestamp.After(*endTime) {
				continue
			}

			_ = s.ChannelMessageDelete(channelID, msg.ID)
			time.Sleep(300 * time.Millisecond)
		}

		lastID = msgs[len(msgs)-1].ID
		if len(msgs) < 100 {
			break
		}
	}
}
