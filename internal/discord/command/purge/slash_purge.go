package purge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/perm"
	"github.com/keshon/server-domme/internal/discord/reply"
	st "github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

type PurgeCommand struct{}

func (c *PurgeCommand) Name() string        { return "purge" }
func (c *PurgeCommand) Description() string { return "Manage message purges" }
func (c *PurgeCommand) Group() string       { return "purge" }
func (c *PurgeCommand) Category() string    { return "🧹 Cleanup" }
func (c *PurgeCommand) UserPermissions() []int64 {
	return []int64{perm.Administrator}
}

func (c *PurgeCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionSubCommand,
				Name:        "auto",
				Description: "Regularly purge old messages in this channel",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionString,
						Name:        "older_than",
						Description: "Purge messages older than this (e.g. 10m, 1h, 1d, 1w)",
						Required:    true,
					},
					{
						Type:        adapter.OptionString,
						Name:        "confirm",
						Description: "Type 'yes' to confirm the action",
						Required:    true,
					},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "now",
				Description: "Schedule or perform an immediate purge",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionString,
						Name:        "delay",
						Description: "Delay before purge starts",
						Required:    true,
						Choices: []adapter.SlashChoice{
							{Name: "Now (no delay)", Value: "0s"},
							{Name: "10 minutes", Value: "10m"},
							{Name: "30 minutes", Value: "30m"},
							{Name: "1 hour", Value: "1h"},
							{Name: "6 hours", Value: "6h"},
							{Name: "1 day", Value: "24h"},
						},
					},
					{
						Type:        adapter.OptionString,
						Name:        "confirm",
						Description: "Type 'yes' to confirm the action",
						Required:    true,
					},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subChannel,
				Description: "Allow or forbid purges in this channel — or see whether they are allowed, left empty",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionBoolean,
						Name:        optAllowed,
						Description: "True: purges may run here · False: forbidden, and any job here is stopped",
					},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "jobs",
				Description: "List all active purge jobs and the channels purges are allowed in",
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "stop",
				Description: "Stop ongoing purge in this channel",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionString,
						Name:        "confirm",
						Description: "Type 'yes' to confirm the action",
						Required:    true,
					},
				},
			},
		},
	}
}

func (c *PurgeCommand) Run(ctx *adapter.SlashInteractionContext) error {
	sub, ok := ctx.FirstOption()
	if !ok {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Please select a subcommand: `auto`, `now`, `channel`, `jobs`, or `stop`.",
			Color:       reply.EmbedColor,
		})
	}

	switch sub.Name {
	case "auto":
		return runPurgeAuto(ctx, sub)
	case "now":
		return runPurgeNow(ctx, sub)
	case subChannel:
		return runPurgeChannel(ctx, sub)
	case "jobs":
		return runPurgeJobs(ctx)
	case "stop":
		return runPurgeStop(ctx, sub)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Unknown subcommand: %s.", sub.Name),
			Color:       reply.EmbedColor,
		})
	}
}

func subString(sub adapter.SlashArgument, name string) string {
	opt, _ := sub.Option(name)
	return opt.StringValue()
}

func runPurgeAuto(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	olderThan := subString(sub, "older_than")
	confirm := subString(sub, "confirm")

	store := ctx.Storage
	if !store.IsPurgeAllowed(ctx.GuildID(), ctx.ChannelID()) {
		return ctx.RespondEphemeral(&adapter.Embed{Description: notAllowed,
			Color: reply.EmbedColor})
	}

	if strings.ToLower(confirm) != "yes" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Action not confirmed. Please type 'yes' to proceed.",
			Color:       reply.EmbedColor,
		})
	}

	dur, err := parseDuration(olderThan)
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Invalid duration format. Use `10m`, `2h`, `1d`, etc.",
			Color:       reply.EmbedColor,
		})
	}

	if !ctx.API.CheckBotPermissions(ctx.ChannelID()) {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Missing permissions to purge messages.",
			Color:       reply.EmbedColor,
		})
	}

	ActiveDeletionsMu.Lock()
	if _, exists := ActiveDeletions[ctx.ChannelID()]; exists {
		ActiveDeletionsMu.Unlock()
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "A purge job is already running in this channel.",
			Color:       reply.EmbedColor,
		})
	}
	stopChan := make(chan struct{})
	ActiveDeletions[ctx.ChannelID()] = stopChan
	ActiveDeletionsMu.Unlock()

	err = store.SetDeletionJob(ctx.GuildID(), ctx.ChannelID(), st.PurgeModeRecurring, time.Now(), true, olderThan)
	if err != nil {
		stopDeletion(ctx.ChannelID())
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to set deletion job: `%v`.", err),
			Color:       reply.EmbedColor,
		})
	}

	_ = ctx.RespondEphemeral(&adapter.Embed{
		Description: "Recurring purge started. Messages older than **" + dur.String() + "** will be erased.",
		Color:       reply.EmbedColor,
	})

	// Purges always warn the channel: deleting history without notice is not
	// something a flag should be able to silence.
	if err := sendNukeWarning(ctx.API, ctx.ChannelID(), &adapter.Embed{
		Title:       "🧹 Recurring Purge Active",
		Description: fmt.Sprintf("Messages older than `%s` are deleted on a schedule.", dur.String()),
		Color:       ctx.API.EmbedColor(),
	}, "assets/purge/nuke-recurring.webp"); err != nil {
		announceFailed(ctx.AppLog, ctx.ChannelID(), err)
	}

	api := ctx.API
	guildID, channelID := ctx.GuildID(), ctx.ChannelID()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer stopDeletion(channelID)

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				if !store.IsPurgeAllowed(guildID, channelID) {
					return
				}
				DeleteOlderThan(api, channelID, dur, stopChan)
			}
		}
	}()
	return nil
}

func runPurgeNow(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	delayStr := subString(sub, "delay")
	confirm := subString(sub, "confirm")

	store := ctx.Storage
	if !store.IsPurgeAllowed(ctx.GuildID(), ctx.ChannelID()) {
		return ctx.RespondEphemeral(&adapter.Embed{Description: notAllowed,
			Color: reply.EmbedColor})
	}

	if strings.ToLower(confirm) != "yes" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Action not confirmed. Please type 'yes' to proceed.",
			Color:       reply.EmbedColor,
		})
	}

	ActiveDeletionsMu.Lock()
	if _, exists := ActiveDeletions[ctx.ChannelID()]; exists {
		ActiveDeletionsMu.Unlock()
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "A purge job is already running in this channel.",
			Color:       reply.EmbedColor,
		})
	}
	ActiveDeletionsMu.Unlock()

	if delayStr == "0s" {
		delayStr = "10s"
	}

	dur, err := parseDuration(delayStr)
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Invalid delay format. Use formats like `10m`, `1h`, `1d`.",
			Color:       reply.EmbedColor,
		})
	}

	delayUntil := time.Now().Add(dur)
	if err := store.SetDeletionJob(ctx.GuildID(), ctx.ChannelID(), st.PurgeModeDelayed, delayUntil, true); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to schedule purge: `%v`.", err),
			Color:       reply.EmbedColor,
		})
	}

	_ = ctx.RespondEphemeral(&adapter.Embed{
		Description: "Purge scheduled — will start in **" + dur.String() + "**.",
		Color:       reply.EmbedColor,
	})

	// The warning always goes out; see runPurgeAuto.
	if err := sendNukeWarning(ctx.API, ctx.ChannelID(), &adapter.Embed{
		Title:       "🧹 Purge Scheduled",
		Description: "All messages in this channel will be deleted in `" + dur.String() + "`.",
		Color:       ctx.API.EmbedColor(),
	}, "assets/purge/nuke-upcoming.gif"); err != nil {
		announceFailed(ctx.AppLog, ctx.ChannelID(), err)
	}

	// Registered for the whole countdown, not only the deleting: /purge stop
	// and /purge channel allowed:false close it.
	stopChan := make(chan struct{})
	ActiveDeletionsMu.Lock()
	ActiveDeletions[ctx.ChannelID()] = stopChan
	ActiveDeletionsMu.Unlock()

	api := ctx.API
	guildID, channelID := ctx.GuildID(), ctx.ChannelID()
	appLog := ctx.AppLog
	go func() {
		timer := time.NewTimer(dur)
		defer timer.Stop()
		select {
		case <-stopChan:
			return
		case <-timer.C:
		}
		if store.IsPurgeAllowed(guildID, channelID) {
			DeleteMessages(api, channelID, nil, nil, stopChan)
		}

		ActiveDeletionsMu.Lock()
		if ActiveDeletions[channelID] == stopChan {
			delete(ActiveDeletions, channelID)
		}
		ActiveDeletionsMu.Unlock()
		if err := store.ClearDeletionJob(guildID, channelID); err != nil {
			appLog.Error().
				Str("channel_id", channelID).
				Err(err).
				Msg("purge_job_clear_failed")
		}
	}()
	return nil
}

func runPurgeJobs(ctx *adapter.SlashInteractionContext) error {
	store := ctx.Storage
	jobs, err := store.GetDeletionJobsList(ctx.GuildID())
	allowed := allowedList(store.GetPurgeChannels(ctx.GuildID()))
	if err != nil || len(jobs) == 0 {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No active purge jobs found.\n\n" + allowed,
			Color:       reply.EmbedColor,
		})
	}

	var sb strings.Builder
	sb.WriteString(allowed + "\n\n")
	sb.WriteString("🧹 **Active Purge Jobs**\n\n")
	for _, job := range jobs {
		sb.WriteString("<#" + job.ChannelID + ">\n")
		switch job.Mode {
		case st.PurgeModeDelayed:
			eta := time.Until(job.DelayUntil).Truncate(time.Second)
			if eta > 0 {
				sb.WriteString("Runs in: `" + eta.String() + "`\n")
			} else {
				sb.WriteString("Overdue by: `" + (-eta).String() + "`\n")
			}
		case st.PurgeModeRecurring:
			sb.WriteString("Recurring purge of messages older than `" + job.OlderThan + "`\n")
		default:
			sb.WriteString("Unknown mode: " + job.Mode + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Use `/purge stop confirm:yes` to cancel any listed job.")
	return ctx.RespondEphemeral(&adapter.Embed{Description: sb.String(),
		Color: reply.EmbedColor})
}

func runPurgeStop(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	if strings.ToLower(subString(sub, "confirm")) != "yes" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Action not confirmed. Please type 'yes' to proceed.",
			Color:       reply.EmbedColor,
		})
	}
	store := ctx.Storage
	stopDeletion(ctx.ChannelID())
	if _, err := store.GetDeletionJob(ctx.GuildID(), ctx.ChannelID()); err == nil {
		_ = store.ClearDeletionJob(ctx.GuildID(), ctx.ChannelID())
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "Message purge job stopped.",
			Color:       reply.EmbedColor,
		})
	} else {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "No active purge job in this channel.",
			Color:       reply.EmbedColor,
		})
	}
	return nil
}

// /purge channel and its option.
const (
	subChannel = "channel"
	optAllowed = "allowed"
)

// notAllowed is the answer to a purge in a channel not on the list.
const notAllowed = "Purges are not allowed in this channel. An administrator allows them here with " +
	"`/purge channel allowed:true` — deliberately a separate step, because a purge cannot be undone."

// runPurgeChannel allows or forbids purges in the channel it is run in, or
// says which it is. Forbidding also stops and clears any job here.
func runPurgeChannel(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	store := ctx.Storage
	opt, ok := sub.Option(optAllowed)
	respond := func(msg string) error {
		return ctx.RespondEphemeral(&adapter.Embed{Description: msg,
			Color: reply.EmbedColor})
	}
	if !ok {
		if store.IsPurgeAllowed(ctx.GuildID(), ctx.ChannelID()) {
			return respond(fmt.Sprintf("Purges are allowed in <#%s>.", ctx.ChannelID()))
		}
		return respond(fmt.Sprintf("Purges are not allowed in <#%s>.", ctx.ChannelID()))
	}
	if opt.BoolValue() {
		if err := store.SetPurgeAllowed(ctx.GuildID(), ctx.ChannelID(), true); err != nil {
			return fmt.Errorf("purge: allow channel: %w", err)
		}
		return respond(fmt.Sprintf("Purges are now allowed in <#%s>. `/purge now` and `/purge auto` work here.", ctx.ChannelID()))
	}
	stopDeletion(ctx.ChannelID())
	if err := store.ClearDeletionJob(ctx.GuildID(), ctx.ChannelID()); err != nil {
		return fmt.Errorf("purge: clear job: %w", err)
	}
	if err := store.SetPurgeAllowed(ctx.GuildID(), ctx.ChannelID(), false); err != nil {
		return fmt.Errorf("purge: forbid channel: %w", err)
	}
	return respond(fmt.Sprintf("Purges are no longer allowed in <#%s>, and any purge job here was stopped.", ctx.ChannelID()))
}

// allowedList says where purges are allowed.
func allowedList(channels []string) string {
	if len(channels) == 0 {
		return "Purges are allowed nowhere yet — `/purge channel allowed:true` in a channel."
	}
	parts := make([]string, 0, len(channels))
	for _, c := range channels {
		parts = append(parts, "<#"+c+">")
	}
	return "Purges are allowed in: " + strings.Join(parts, ", ")
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
		return 0, errors.New("purge: invalid duration format")
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
			return 0, errors.New("purge: unknown time unit: " + unit)
		}
	}

	return total, nil
}

// DeleteOlderThan deletes a channel's messages older than age: what a
// recurring purge does on each tick.
//
// One place for it, because the two callers had the window wrong in opposite
// ways. DeleteMessages keeps what falls between its start and end; /purge auto
// passed them reversed and deleted nothing, and the scheduler passed
// [now-age, now] and deleted the newest messages instead of the oldest — a
// recurring "older than 1d" purge, replayed after a restart, wiped the last
// day of the channel every thirty seconds.
func DeleteOlderThan(api adapter.SessionAPI, channelID string, age time.Duration, stopChan <-chan struct{}) {
	cutoff := time.Now().Add(-age)
	DeleteMessages(api, channelID, nil, &cutoff, stopChan)
}

func DeleteMessages(api adapter.SessionAPI, channelID string, startTime, endTime *time.Time, stopChan <-chan struct{}) {
	var lastID string

	for {
		select {
		case <-stopChan:
			return
		default:
		}

		msgs, err := api.ChannelMessages(channelID, lastID, 100)
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

			_ = api.DeleteMessage(channelID, msg.ID)
			time.Sleep(300 * time.Millisecond)
		}

		lastID = msgs[len(msgs)-1].ID
		if len(msgs) < 100 {
			break
		}
	}
}

// sendNukeWarning posts a purge warning with its gif attached from the local
// assets, so the warning survives the source link dying (third-party gif
// hosts are not to be trusted). When the file cannot be opened it falls back
// to the embed alone rather than failing the scheduling it announces.
func sendNukeWarning(api adapter.SessionAPI, channelID string, embed *adapter.Embed, assetPath string) error {
	f, err := os.Open(assetPath)
	if err != nil {
		return api.SendChannelEmbed(channelID, embed)
	}
	defer f.Close()
	embed.ImageURL = "attachment://" + filepath.Base(assetPath)
	return api.SendChannelEmbedFile(channelID, embed, f, filepath.Base(assetPath))
}

// announceFailed logs a channel-wide purge warning that could not be posted,
// instead of propagating it. The purge itself is already scheduled by this
// point, so a missing warning must not read back to the invoker as "the purge
// did not start".
func announceFailed(log zerolog.Logger, channelID string, err error) {
	log.Warn().
		Str("channel_id", channelID).
		Err(err).
		Msg("purge_announce_failed")
}
