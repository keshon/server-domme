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
func (c *PurgeAutoCommand) RequireDev() bool    { return false }

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

	guildID := event.GuildID
	member := event.Member

	if !core.CheckBotPermissions(session, event.ChannelID) {
		core.RespondEphemeral(session, event, "Missing permissions to purge messages in this channel.")
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
		core.RespondEphemeral(session, event, "Action not confirmed. Please type 'yes' to proceed.")
		return nil
	}

	dur, err := parseDuration(olderThan)
	if err != nil {
		core.RespondEphemeral(session, event, "Invalid duration format. Use values like `10m`, `2h`, `1d`, `1w` etc.")
		return nil
	}

	ActiveDeletionsMu.Lock()
	if _, exists := ActiveDeletions[event.ChannelID]; exists {
		ActiveDeletionsMu.Unlock()
		core.RespondEphemeral(session, event, "This channel is already undergoing recurring purge.")
		return nil
	}
	stopChan := make(chan struct{})
	ActiveDeletions[event.ChannelID] = stopChan
	ActiveDeletionsMu.Unlock()

	err = storage.SetDeletionJob(event.GuildID, event.ChannelID, "recurring", time.Now(), notifyAll, olderThan)
	if err != nil {
		stopDeletion(event.ChannelID)
		core.RespondEphemeral(session, event, "Failed to schedule recurring purge: "+err.Error())
		return nil
	}

	core.RespondEphemeral(session, event, "Recurring message purge started.\nMessages older than **"+dur.String()+"** will be removed.")

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
		_, _ = session.ChannelMessageSendEmbed(event.ChannelID, embed)
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

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&PurgeAutoCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
		),
	)
}
