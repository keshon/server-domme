package command

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PurgeAutoCommand struct{}

func (c *PurgeAutoCommand) Name() string        { return "purge-auto" }
func (c *PurgeAutoCommand) Description() string { return "Recurring purge of messages older than set" }
func (c *PurgeAutoCommand) Aliases() []string   { return []string{} }

func (c *PurgeAutoCommand) Group() string    { return "purge" }
func (c *PurgeAutoCommand) Category() string { return "🧹 Channel Cleanup" }

func (c *PurgeAutoCommand) RequireAdmin() bool { return true }
func (c *PurgeAutoCommand) RequireDev() bool   { return false }

func (c *PurgeAutoCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "older_than",
				Description: "Purge messages older than this duration (e.g. 10m, 1h, 1d, 1w)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "notify_all",
				Description: "Post a notification message",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Yes (default)", Value: "true"},
					{Name: "No", Value: "false"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "confirm",
				Description: "Type 'yes' to confirm the action",
				Required:    true,
			},
		},
	}
}

func (c *PurgeAutoCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	s := slash.Session
	i := slash.Event
	storage := slash.Storage

	if !checkBotPermissions(s, i.ChannelID) {
		respondEphemeral(s, i, "Missing permissions to purge messages in this channel.")
		return nil
	}

	var confirm, olderThan string
	var notifyAll bool

	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "confirm":
			confirm = opt.StringValue()
		case "older_than":
			olderThan = opt.StringValue()
		case "notify_all":
			notifyAll = strings.ToLower(opt.StringValue()) == "true"
		}
	}

	if strings.ToLower(confirm) != "yes" {
		respondEphemeral(s, i, "Action not confirmed. Please type 'yes' to proceed.")
		return nil
	}

	dur, err := parseDuration(olderThan)
	if err != nil {
		respondEphemeral(s, i, "Invalid duration format. Use values like `10m`, `2h`, `1d`, `1w` etc.")
		return nil
	}

	ActiveDeletionsMu.Lock()
	if _, exists := ActiveDeletions[i.ChannelID]; exists {
		ActiveDeletionsMu.Unlock()
		respondEphemeral(s, i, "This channel is already undergoing recurring purge.")
		return nil
	}
	stopChan := make(chan struct{})
	ActiveDeletions[i.ChannelID] = stopChan
	ActiveDeletionsMu.Unlock()

	err = storage.SetDeletionJob(i.GuildID, i.ChannelID, "recurring", time.Now(), notifyAll, olderThan)
	if err != nil {
		stopDeletion(i.ChannelID)
		respondEphemeral(s, i, "Failed to schedule recurring purge: "+err.Error())
		return nil
	}

	respondEphemeral(s, i, "Recurring message purge started.\nMessages older than **"+dur.String()+"** will be removed.")

	if notifyAll {
		imgURL := "https://ichef.bbci.co.uk/images/ic/1376xn/p05cj1tt.jpg.webp"
		embed := &discordgo.MessageEmbed{
			Title:       "☢️ Recurring Nuke Detonation",
			Description: "This channel is now under a standing nuke order.\nAny messages older than `" + dur.String() + "` will be *systematically erased*.",
			Color:       embedColor,
			Image:       &discordgo.MessageEmbedImage{URL: imgURL},
			Footer:      &discordgo.MessageEmbedFooter{Text: "History has a half-life."},
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		_, _ = s.ChannelMessageSendEmbed(i.ChannelID, embed)
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer stopDeletion(i.ChannelID)

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				start := time.Now().Add(-dur)
				now := time.Now()
				DeleteMessages(s, i.ChannelID, &now, &start, stopChan)
			}
		}
	}()

	logErr := logCommand(s, slash.Storage, i.GuildID, i.ChannelID, i.Member.User.ID, i.Member.User.Username, "purge-auto")
	if logErr != nil {
		log.Println("Failed to log command:", logErr)
	}

	return nil
}

func init() {
	Register(WithGuildOnly(&PurgeAutoCommand{}))
}
