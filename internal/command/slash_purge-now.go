package command

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PurgeNowCommand struct{}

func (c *PurgeNowCommand) Name() string        { return "purge-now" }
func (c *PurgeNowCommand) Description() string { return "Purge messages in this channel" }
func (c *PurgeNowCommand) Aliases() []string   { return []string{} }

func (c *PurgeNowCommand) Group() string    { return "purge" }
func (c *PurgeNowCommand) Category() string { return "🧹 Cleanup" }

func (c *PurgeNowCommand) RequireAdmin() bool { return true }
func (c *PurgeNowCommand) RequireDev() bool   { return false }

func (c *PurgeNowCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "delay",
				Description: "Delay before purge starts",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Now (no delay)", Value: "0s"},
					{Name: "10 minutes", Value: "10m"},
					{Name: "30 minutes", Value: "30m"},
					{Name: "1 hour", Value: "1h"},
					{Name: "6 hours", Value: "6h"},
					{Name: "1 day", Value: "24h"},
				},
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

func (c *PurgeNowCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	s := slash.Session
	i := slash.Event
	storage := slash.Storage

	if !checkBotPermissions(s, i.ChannelID) {
		respondEphemeral(s, i, "Missing permissions to delete messages in this channel.")
		return nil
	}

	var delayStr, confirm string
	var notifyAll bool

	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "delay":
			delayStr = opt.StringValue()
		case "confirm":
			confirm = opt.StringValue()
		case "notify_all":
			notifyAll = strings.ToLower(opt.StringValue()) == "true"
		}
	}

	if strings.ToLower(confirm) != "yes" {
		respondEphemeral(s, i, "Action not confirmed. Please type 'yes' to proceed.")
		return nil
	}

	if delayStr == "0s" {
		delayStr = "10s"
	}

	dur, err := parseDuration(delayStr)
	if err != nil {
		respondEphemeral(s, i, "Invalid delay format. Use formats like `10m`, `1h`, `1d`.")
		return nil
	}

	delayUntil := time.Now().Add(dur)
	if err := storage.SetDeletionJob(i.GuildID, i.ChannelID, "delayed", delayUntil, notifyAll); err != nil {
		respondEphemeral(s, i, "Failed to schedule purge: "+err.Error())
		return nil
	}

	respondEphemeral(s, i, "Message purge scheduled.\nThis channel will be purged in **"+dur.String()+"**.")

	if notifyAll {
		embed := &discordgo.MessageEmbed{
			Title:       "☢️ Upcoming Nuke Detonation",
			Description: "Countdown initiated.\nAll messages in this channel will be *obliterated* in `" + dur.String() + "`.\nPrepare for impact.",
			Color:       embedColor,
			Image:       &discordgo.MessageEmbedImage{URL: "https://c.tenor.com/qDvLEFO5bAkAAAAd/tenor.gif"},
			Footer:      &discordgo.MessageEmbedFooter{Text: "May your sins be incinerated."},
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		_, _ = s.ChannelMessageSendEmbed(i.ChannelID, embed)
	}

	go func() {
		time.Sleep(dur)

		stopChan := make(chan struct{})
		ActiveDeletionsMu.Lock()
		ActiveDeletions[i.ChannelID] = stopChan
		ActiveDeletionsMu.Unlock()

		DeleteMessages(s, i.ChannelID, nil, nil, stopChan)

		ActiveDeletionsMu.Lock()
		delete(ActiveDeletions, i.ChannelID)
		ActiveDeletionsMu.Unlock()

		_ = storage.ClearDeletionJob(i.GuildID, i.ChannelID)
	}()

	logErr := logCommand(s, storage, i.GuildID, i.ChannelID, i.Member.User.ID, i.Member.User.Username, "purge-now")
	if logErr != nil {
		log.Println("Failed to log command:", logErr)
	}
	return nil
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&PurgeNowCommand{},
			),
		),
	)
}
