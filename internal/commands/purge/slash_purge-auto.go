package purge

import (
	"log"
	"server-domme/internal/core"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PurgeAutoCommand struct{}

func (c *PurgeAutoCommand) Name() string        { return "purge-auto" }
func (c *PurgeAutoCommand) Description() string { return "Purge messages regularly in this channel" }
func (c *PurgeAutoCommand) Aliases() []string   { return []string{} }
func (c *PurgeAutoCommand) Group() string       { return "purge" }
func (c *PurgeAutoCommand) Category() string    { return "🧹 Cleanup" }
func (c *PurgeAutoCommand) RequireAdmin() bool  { return true }
func (c *PurgeAutoCommand) Permissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}

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
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	if !core.CheckBotPermissions(session, event.ChannelID) {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Missing permissions to purge messages in this channel.",
		})
		return nil
	}

	var confirm, olderThan string
	var notifyAll bool

	for _, opt := range event.ApplicationCommandData().Options {
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
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Action not confirmed. Please type 'yes' to proceed.",
		})
		return nil
	}

	dur, err := parseDuration(olderThan)
	if err != nil {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Invalid duration format. Use values like `10m`, `2h`, `1d`, `1w` etc.",
		})
		return nil
	}

	ActiveDeletionsMu.Lock()
	if _, exists := ActiveDeletions[event.ChannelID]; exists {
		ActiveDeletionsMu.Unlock()
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "A purge job is already running in this channel.",
		})
		return nil
	}
	stopChan := make(chan struct{})
	ActiveDeletions[event.ChannelID] = stopChan
	ActiveDeletionsMu.Unlock()

	err = storage.SetDeletionJob(event.GuildID, event.ChannelID, "recurring", time.Now(), notifyAll, olderThan)
	if err != nil {
		stopDeletion(event.ChannelID)
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Failed to set deletion job: " + err.Error(),
		})
		return nil
	}

	core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
		Description: "Recurring message purge started.\nMessages older than **" + dur.String() + "** will be removed.",
	})

	if notifyAll {
		imgURL := "https://ichef.bbci.co.uk/images/ic/1376xn/p05cj1tt.jpg.webp"
		embed := &discordgo.MessageEmbed{
			Title:       "☢️ Recurring Nuke Detonation",
			Description: "This channel is now under a standing nuke order.\nAny messages older than `" + dur.String() + "` will be *systematically erased*.",
			Color:       core.EmbedColor,
			Image:       &discordgo.MessageEmbedImage{URL: imgURL},
			Footer:      &discordgo.MessageEmbedFooter{Text: "History has a half-life."},
			Timestamp:   time.Now().Format(time.RFC3339),
		}

		// Use ChannelMessageSend for public messages instead of InteractionRespond (we used RespondEmbedEphemeral earlier once already)
		_, err := session.ChannelMessageSendEmbed(event.ChannelID, embed)
		if err != nil {
			log.Println("Failed to send public notification:", err)
		}
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer stopDeletion(event.ChannelID)

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				start := time.Now().Add(-dur)
				now := time.Now()
				DeleteMessages(session, event.ChannelID, &now, &start, stopChan)
			}
		}
	}()

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&PurgeAutoCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithCommandLogger(),
		),
	)
}
