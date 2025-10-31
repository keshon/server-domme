package purge

import (
	"errors"
	"fmt"
	"regexp"
	"server-domme/internal/bot"
	"server-domme/internal/middleware"
	"server-domme/internal/registry"

	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PurgeCommand struct{}

func (c *PurgeCommand) Name() string        { return "purge" }
func (c *PurgeCommand) Description() string { return "Manage message purges" }
func (c *PurgeCommand) Group() string       { return "purge" }
func (c *PurgeCommand) Category() string    { return "🧹 Cleanup" }
func (c *PurgeCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *PurgeCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "auto",
				Description: "Regularly purge old messages in this channel",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "older_than",
						Description: "Purge messages older than this (e.g. 10m, 1h, 1d, 1w)",
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
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "now",
				Description: "Schedule or perform an immediate purge",
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
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "jobs",
				Description: "List all active purge jobs",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "stop",
				Description: "Stop ongoing purge in this channel",
			},
		},
	}
}

func (c *PurgeCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*registry.SlashInteractionContext)
	if !ok {
		return nil
	}

	event := context.Event
	session := context.Session

	data := event.ApplicationCommandData()
	if len(data.Options) == 0 {
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Please select a subcommand: `auto`, `now`, `jobs`, or `stop`.",
		})
	}

	sub := data.Options[0]
	switch sub.Name {
	case "auto":
		return runPurgeAuto(context, sub)
	case "now":
		return runPurgeNow(context, sub)
	case "jobs":
		return runPurgeJobs(context)
	case "stop":
		return runPurgeStop(context)
	default:
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

func runPurgeAuto(ctx *registry.SlashInteractionContext, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	session := ctx.Session
	event := ctx.Event
	storage := ctx.Storage

	var olderThan, confirm string
	var notifyAll bool

	for _, opt := range sub.Options {
		switch opt.Name {
		case "older_than":
			olderThan = opt.StringValue()
		case "confirm":
			confirm = opt.StringValue()
		case "notify_all":
			notifyAll = strings.ToLower(opt.StringValue()) == "true"
		}
	}

	if strings.ToLower(confirm) != "yes" {
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Action not confirmed. Please type 'yes' to proceed.",
		})
	}

	dur, err := parseDuration(olderThan)
	if err != nil {
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Invalid duration format. Use `10m`, `2h`, `1d`, etc.",
		})
	}

	if !bot.CheckBotPermissions(session, event.ChannelID) {
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Missing permissions to purge messages.",
		})
	}

	ActiveDeletionsMu.Lock()
	if _, exists := ActiveDeletions[event.ChannelID]; exists {
		ActiveDeletionsMu.Unlock()
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "A purge job is already running in this channel.",
		})
	}
	stopChan := make(chan struct{})
	ActiveDeletions[event.ChannelID] = stopChan
	ActiveDeletionsMu.Unlock()

	err = storage.SetDeletionJob(event.GuildID, event.ChannelID, "recurring", time.Now(), notifyAll, olderThan)
	if err != nil {
		stopDeletion(event.ChannelID)
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Failed to set deletion job: " + err.Error(),
		})
	}

	bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
		Description: "Recurring purge started. Messages older than **" + dur.String() + "** will be erased.",
	})

	if notifyAll {
		session.ChannelMessageSendEmbed(event.ChannelID, &discordgo.MessageEmbed{
			Title:       "☢️ Recurring Nuke Detonation",
			Description: fmt.Sprintf("All messages older than `%s` will be **systematically erased**.", dur.String()),
			Color:       bot.EmbedColor,
			Image:       &discordgo.MessageEmbedImage{URL: "https://ichef.bbci.co.uk/images/ic/1376xn/p05cj1tt.jpg.webp"},
			Footer:      &discordgo.MessageEmbedFooter{Text: "History has a half-life."},
		})
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

func runPurgeNow(ctx *registry.SlashInteractionContext, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	session := ctx.Session
	event := ctx.Event
	storage := ctx.Storage

	var delayStr, confirm string
	var notifyAll bool
	for _, opt := range sub.Options {
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
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Action not confirmed. Please type 'yes' to proceed.",
		})
	}

	if delayStr == "0s" {
		delayStr = "10s"
	}

	dur, err := parseDuration(delayStr)
	if err != nil {
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Invalid delay format. Use formats like `10m`, `1h`, `1d`.",
		})
	}

	delayUntil := time.Now().Add(dur)
	if err := storage.SetDeletionJob(event.GuildID, event.ChannelID, "delayed", delayUntil, notifyAll); err != nil {
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Failed to schedule purge: " + err.Error(),
		})
	}

	bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
		Description: "Purge scheduled — will start in **" + dur.String() + "**.",
	})

	if notifyAll {
		session.ChannelMessageSendEmbed(event.ChannelID, &discordgo.MessageEmbed{
			Title:       "☢️ Upcoming Nuke Detonation",
			Description: "Countdown initiated — all messages will be purged in `" + dur.String() + "`.",
			Color:       bot.EmbedColor,
			Image:       &discordgo.MessageEmbedImage{URL: "https://c.tenor.com/qDvLEFO5bAkAAAAd/tenor.gif"},
			Footer:      &discordgo.MessageEmbedFooter{Text: "May your sins be incinerated."},
		})
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
		storage.ClearDeletionJob(event.GuildID, event.ChannelID)
	}()
	return nil
}

func runPurgeJobs(ctx *registry.SlashInteractionContext) error {
	session := ctx.Session
	event := ctx.Event
	storage := ctx.Storage

	jobs, err := storage.GetDeletionJobsList(event.GuildID)
	if err != nil || len(jobs) == 0 {
		return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No active purge jobs found.",
		})
	}

	var sb strings.Builder
	sb.WriteString("☢️ **Active Message Purge Jobs**\n\n")
	for _, job := range jobs {
		sb.WriteString("<#" + job.ChannelID + ">\n")
		switch job.Mode {
		case "delayed":
			eta := time.Until(job.DelayUntil).Truncate(time.Second)
			if eta > 0 {
				sb.WriteString("Runs in: `" + eta.String() + "`\n")
			} else {
				sb.WriteString("Overdue by: `" + (-eta).String() + "`\n")
			}
		case "recurring":
			sb.WriteString("Recurring purge of messages older than `" + job.OlderThan + "`\n")
		default:
			sb.WriteString("Unknown mode: " + job.Mode + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Use `/purge stop` to cancel any listed job.")
	return bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{Description: sb.String()})
}

func runPurgeStop(ctx *registry.SlashInteractionContext) error {
	session := ctx.Session
	event := ctx.Event
	storage := ctx.Storage

	stopDeletion(event.ChannelID)
	if _, err := storage.GetDeletionJob(event.GuildID, event.ChannelID); err == nil {
		_ = storage.ClearDeletionJob(event.GuildID, event.ChannelID)
		bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Message purge job stopped.",
		})
	} else {
		bot.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No active purge job in this channel.",
		})
	}
	return nil
}

var (
	ActiveDeletions   = make(map[string]chan struct{})
	ActiveDeletionsMu sync.Mutex

	timePattern = regexp.MustCompile(`(?i)(\d+)([smhdw])`)
)

func stopDeletion(channelID string) {
	ActiveDeletionsMu.Lock()
	defer ActiveDeletionsMu.Unlock()
	if ch, ok := ActiveDeletions[channelID]; ok {
		close(ch)
		delete(ActiveDeletions, channelID)
	}
}

func parseDuration(input string) (time.Duration, error) {
	matches := timePattern.FindAllStringSubmatch(input, -1)
	if matches == nil {
		return 0, errors.New("invalid duration format")
	}

	var total time.Duration
	for _, match := range matches {
		value, _ := strconv.Atoi(match[1])
		unit := match[2]

		switch unit {
		case "s":
			total += time.Duration(value) * time.Second
		case "m":
			total += time.Duration(value) * time.Minute
		case "h":
			total += time.Duration(value) * time.Hour
		case "d":
			total += time.Duration(value) * 24 * time.Hour
		case "w":
			total += time.Duration(value) * 7 * 24 * time.Hour
		default:
			return 0, errors.New("unknown time unit: " + unit)
		}
	}

	return total, nil
}

func DeleteMessages(s *discordgo.Session, channelID string, startTime, endTime *time.Time, stopChan <-chan struct{}) {
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

func init() {
	registry.RegisterCommand(
		middleware.ApplyMiddlewares(
			&PurgeCommand{},
			middleware.WithGroupAccessCheck(),
			middleware.WithGuildOnly(),
			middleware.WithUserPermissionCheck(),
			middleware.WithCommandLogger(),
		),
	)
}
