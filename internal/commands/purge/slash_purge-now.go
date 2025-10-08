package purge

import (
	"log"
	"server-domme/internal/core"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PurgeNowCommand struct{}

func (c *PurgeNowCommand) Name() string        { return "purge-now" }
func (c *PurgeNowCommand) Description() string { return "Purge messages in this channel" }
func (c *PurgeNowCommand) Aliases() []string   { return []string{} }
func (c *PurgeNowCommand) Group() string       { return "purge" }
func (c *PurgeNowCommand) Category() string    { return "🧹 Cleanup" }
func (c *PurgeNowCommand) RequireAdmin() bool  { return true }
func (c *PurgeNowCommand) RequireDev() bool    { return false }

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
		core.RespondEphemeral(session, event, "Missing permissions to delete messages in this channel.")
		return nil
	}

	var delayStr, confirm string
	var notifyAll bool

	for _, opt := range event.ApplicationCommandData().Options {
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
		core.RespondEphemeral(session, event, "Action not confirmed. Please type 'yes' to proceed.")
		return nil
	}

	if delayStr == "0s" {
		delayStr = "10s"
	}

	dur, err := parseDuration(delayStr)
	if err != nil {
		core.RespondEphemeral(session, event, "Invalid delay format. Use formats like `10m`, `1h`, `1d`.")
		return nil
	}

	delayUntil := time.Now().Add(dur)
	if err := storage.SetDeletionJob(event.GuildID, event.ChannelID, "delayed", delayUntil, notifyAll); err != nil {
		core.RespondEphemeral(session, event, "Failed to schedule purge: "+err.Error())
		return nil
	}

	core.RespondEphemeral(session, event, "Message purge scheduled.\nThis channel will be purged in **"+dur.String()+"**.")

	if notifyAll {
		embed := &discordgo.MessageEmbed{
			Title:       "☢️ Upcoming Nuke Detonation",
			Description: "Countdown initiated.\nAll messages in this channel will be *obliterated* in `" + dur.String() + "`.\nPrepare for impact.",
			Color:       core.EmbedColor,
			Image:       &discordgo.MessageEmbedImage{URL: "https://c.tenor.com/qDvLEFO5bAkAAAAd/tenor.gif"},
			Footer:      &discordgo.MessageEmbedFooter{Text: "May your sins be incinerated."},
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		_, _ = session.ChannelMessageSendEmbed(event.ChannelID, embed)
	}

	go func() {
		time.Sleep(dur)

		stopChan := make(chan struct{})
		ActiveDeletionsMu.Lock()
		ActiveDeletions[event.ChannelID] = stopChan
		ActiveDeletionsMu.Unlock()

		DeleteMessages(session, event.ChannelID, nil, nil, stopChan)

		ActiveDeletionsMu.Lock()
		delete(ActiveDeletions, event.ChannelID)
		ActiveDeletionsMu.Unlock()

		_ = storage.ClearDeletionJob(event.GuildID, event.ChannelID)
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
			&PurgeNowCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
		),
	)
}
