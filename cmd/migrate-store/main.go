// cmd/migrate-store converts a pre-v1 datastore.json into the directory-based
// store the bot now opens.
//
// The old format was one JSON object per guild holding every feature's state in
// one blob; the new one is typed collections in a write-ahead log. This reads
// the former and writes the latter, and is meant to be run once, with the bot
// stopped — the datastore locks its directory, so a running bot would block it.
//
// It never touches the input file. If the result looks wrong, delete the output
// directory and run it again.
//
//	go run ./cmd/migrate-store -in ./data/datastore.json -out ./data/store
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// legacyRecord is the old per-guild blob. Fields the current schema dropped
// (command_hashes, channels) are deliberately absent: they were unused, and
// decoding ignores what it has no field for.
type legacyRecord struct {
	AnnounceChannel      string                    `json:"announce_channel"`
	ConfessChannel       string                    `json:"confess_channel"`
	CommandsDisabled     []string                  `json:"commands_disabled"`
	CommandsHistory      []legacyCommand           `json:"commands_history"`
	DisciplineRoles      map[string]string         `json:"discipline_roles"`
	MediaCategories      []string                  `json:"media_categories"`
	MediaDefault         string                    `json:"media_default"`
	PurgeJobs            map[string]legacyPurgeJob `json:"purge_jobs"`
	ShortLinks           []legacyShortLink         `json:"short_links"`
	TaskCooldowns        map[string]time.Time      `json:"task_cooldowns"`
	TaskCooldownDuration string                    `json:"task_cooldown_duration"`
	TaskList             map[string]legacyTask     `json:"task_list"`
	TaskRole             string                    `json:"task_role"`
	TranslateChannels    []string                  `json:"translate_channels"`
}

type legacyCommand struct {
	ChannelID   string    `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	GuildName   string    `json:"guild_name"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Command     string    `json:"command"`
	Datetime    time.Time `json:"datetime"`
}

type legacyPurgeJob struct {
	ChannelID  string    `json:"channel_id"`
	GuildID    string    `json:"guild_id"`
	Mode       string    `json:"mode"`
	DelayUntil time.Time `json:"delay_until"`
	OlderThan  string    `json:"older_than"`
	StartedAt  time.Time `json:"started_at"`
	Silent     bool      `json:"silent"`
}

type legacyShortLink struct {
	ShortID  string    `json:"short_id"`
	Original string    `json:"original"`
	UserID   string    `json:"user_id"`
	Created  time.Time `json:"created"`
	Clicks   int       `json:"clicks"`
}

type legacyTask struct {
	UserID     string    `json:"user_id"`
	MessageID  string    `json:"task_message_id"`
	AssignedAt time.Time `json:"assigned_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Status     string    `json:"status"`
}

func main() {
	in := flag.String("in", "./data/datastore.json", "path to the legacy datastore.json")
	out := flag.String("out", "./data/store", "directory to create the new store in")
	verify := flag.Bool("verify", false, "compare an already-migrated store against the legacy file instead of migrating")
	flag.Parse()

	log := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()

	action, verb := run, "migration_failed"
	if *verify {
		action, verb = runVerify, "verification_failed"
	}
	if err := action(*in, *out, log); err != nil {
		log.Error().Str("action", verb).Err(err).Msg("migrate_store_failed")
		os.Exit(1)
	}
}

// runVerify re-reads a migrated store and checks it against the legacy file it
// came from. It is a separate pass rather than a step inside the migration so
// it exercises the same read paths the bot uses, on a store that was closed and
// reopened — which is what actually proves the data landed.
func runVerify(in, out string, log zerolog.Logger) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("migrate-store: read legacy store: %w", err)
	}
	var records map[string]legacyRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Errorf("migrate-store: decode legacy store: %w", err)
	}

	store, err := storage.NewStorage(out, log)
	if err != nil {
		return fmt.Errorf("migrate-store: open migrated store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Error().Err(err).Msg("store_close_failed")
		}
	}()

	problems := 0
	report := func(guildID, what string, want, got any) {
		problems++
		log.Error().Str("guild_id", guildID).Str("field", what).
			Interface("want", want).Interface("got", got).Msg("mismatch")
	}

	for guildID, rec := range records {
		if guildID == "" {
			continue // intentionally not carried over; see run()
		}
		export, err := store.ExportGuild(guildID)
		if err != nil {
			return fmt.Errorf("migrate-store: export guild %s: %w", guildID, err)
		}

		if export.Settings.AnnounceChannel != rec.AnnounceChannel {
			report(guildID, "announce_channel", rec.AnnounceChannel, export.Settings.AnnounceChannel)
		}
		if export.Settings.ConfessChannel != rec.ConfessChannel {
			report(guildID, "confess_channel", rec.ConfessChannel, export.Settings.ConfessChannel)
		}
		if export.Settings.TaskRole != rec.TaskRole {
			report(guildID, "task_role", rec.TaskRole, export.Settings.TaskRole)
		}
		if export.Settings.MediaDefault != rec.MediaDefault {
			report(guildID, "media_default", rec.MediaDefault, export.Settings.MediaDefault)
		}
		if export.Settings.TaskCooldownDuration != rec.TaskCooldownDuration {
			report(guildID, "task_cooldown_duration", rec.TaskCooldownDuration, export.Settings.TaskCooldownDuration)
		}
		if len(export.Settings.CommandsDisabled) != len(rec.CommandsDisabled) {
			report(guildID, "commands_disabled", len(rec.CommandsDisabled), len(export.Settings.CommandsDisabled))
		}
		if len(export.Settings.DisciplineRoles) != len(rec.DisciplineRoles) {
			report(guildID, "discipline_roles", len(rec.DisciplineRoles), len(export.Settings.DisciplineRoles))
		}
		if len(export.Settings.MediaCategories) != len(rec.MediaCategories) {
			report(guildID, "media_categories", len(rec.MediaCategories), len(export.Settings.MediaCategories))
		}
		if len(export.Settings.TranslateChannels) != len(rec.TranslateChannels) {
			report(guildID, "translate_channels", len(rec.TranslateChannels), len(export.Settings.TranslateChannels))
		}

		// The command log is capped, so the expectation is the newest N, not all
		// of them — and the newest row must be the one the legacy file ended on.
		wantLog := min(len(rec.CommandsHistory), commandHistoryLimit)
		if len(export.CommandLog) != wantLog {
			report(guildID, "command_log_count", wantLog, len(export.CommandLog))
		} else if wantLog > 0 {
			newest := rec.CommandsHistory[len(rec.CommandsHistory)-1]
			got := export.CommandLog[len(export.CommandLog)-1]
			if got.Command != newest.Command || !got.Datetime.Equal(newest.Datetime) {
				report(guildID, "command_log_newest", newest.Command, got.Command)
			}
		}

		if len(export.PurgeJobs) != len(rec.PurgeJobs) {
			report(guildID, "purge_jobs", len(rec.PurgeJobs), len(export.PurgeJobs))
		}
		if len(export.Tasks) != len(rec.TaskList) {
			report(guildID, "task_list", len(rec.TaskList), len(export.Tasks))
		}
		if len(export.TaskCooldowns) != len(rec.TaskCooldowns) {
			report(guildID, "task_cooldowns", len(rec.TaskCooldowns), len(export.TaskCooldowns))
		}

		if len(export.ShortLinks) != len(rec.ShortLinks) {
			report(guildID, "short_links", len(rec.ShortLinks), len(export.ShortLinks))
		}
		// Short links are what a stranger on the internet resolves, so check the
		// target of each one rather than just the count.
		for _, want := range rec.ShortLinks {
			gotGuild, got, err := store.FindLinkByID(want.ShortID)
			if err != nil {
				report(guildID, "short_link_missing", want.ShortID, err.Error())
				continue
			}
			if gotGuild != guildID || got.Original != want.Original || got.Clicks != want.Clicks {
				report(guildID, "short_link_"+want.ShortID, want.Original, got.Original)
			}
		}
	}

	if problems > 0 {
		return fmt.Errorf("migrate-store: %d mismatch(es) found", problems)
	}
	log.Info().Int("guilds", len(records)).Str("store", out).Msg("verification_passed")
	return nil
}

// commandHistoryLimit mirrors the storage package's per-guild cap, which the
// migration applies on the way through.
const commandHistoryLimit = 50

func run(in, out string, log zerolog.Logger) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("migrate-store: read legacy store: %w", err)
	}

	var records map[string]legacyRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Errorf("migrate-store: decode legacy store: %w", err)
	}

	// Refuse an existing directory rather than merging into it: a second run
	// would duplicate every command-log row, since those get fresh ids.
	if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("migrate-store: output directory %s already exists — remove it first", out)
	}

	store, err := storage.NewStorage(out, log)
	if err != nil {
		return fmt.Errorf("migrate-store: open new store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Error().Err(err).Msg("store_close_failed")
		}
	}()

	var counts struct{ guilds, commands, purges, links, tasks, cooldowns int }

	for guildID, rec := range records {
		// A record keyed by "" cannot be carried over, and dropping it silently
		// would be the wrong kind of quiet. The datastore rejects an empty key
		// outright (ErrEmptyKey), and rows whose guild term is empty are left
		// out of the by-guild index — so anything written under it would be
		// unreachable by every read path the bot has. These records come from
		// invocations that carried no guild id at all.
		if guildID == "" {
			log.Warn().
				Int("command_log_rows", len(rec.CommandsHistory)).
				Msg("skipped_record_with_empty_guild_id")
			continue
		}
		if err := migrateGuild(store, guildID, rec, &counts, log); err != nil {
			return fmt.Errorf("migrate-store: guild %s: %w", guildID, err)
		}
		counts.guilds++
	}

	log.Info().
		Int("guilds", counts.guilds).
		Int("command_log", counts.commands).
		Int("purge_jobs", counts.purges).
		Int("short_links", counts.links).
		Int("tasks", counts.tasks).
		Int("task_cooldowns", counts.cooldowns).
		Str("out", out).
		Msg("migration_complete")
	return nil
}

func migrateGuild(store *storage.Storage, guildID string, rec legacyRecord, counts *struct{ guilds, commands, purges, links, tasks, cooldowns int }, log zerolog.Logger) error {
	export := storage.GuildExport{
		GuildID: guildID,
		Settings: storage.GuildSettings{
			GuildID:              guildID,
			AnnounceChannel:      rec.AnnounceChannel,
			ConfessChannel:       rec.ConfessChannel,
			CommandsDisabled:     rec.CommandsDisabled,
			DisciplineRoles:      rec.DisciplineRoles,
			MediaCategories:      rec.MediaCategories,
			MediaDefault:         rec.MediaDefault,
			TaskCooldownDuration: rec.TaskCooldownDuration,
			TaskRole:             rec.TaskRole,
			TranslateChannels:    rec.TranslateChannels,
		},
	}

	for _, c := range rec.CommandsHistory {
		export.CommandLog = append(export.CommandLog, storage.CommandLogEntry{
			GuildID:     guildID,
			ChannelID:   c.ChannelID,
			ChannelName: c.ChannelName,
			GuildName:   c.GuildName,
			UserID:      c.UserID,
			Username:    c.Username,
			Command:     c.Command,
			Datetime:    c.Datetime,
		})
	}
	counts.commands += min(len(export.CommandLog), commandHistoryLimit)

	for channelID, j := range rec.PurgeJobs {
		if j.ChannelID == "" {
			j.ChannelID = channelID
		}
		export.PurgeJobs = append(export.PurgeJobs, storage.PurgeJob{
			GuildID:    guildID,
			ChannelID:  j.ChannelID,
			Mode:       j.Mode,
			DelayUntil: j.DelayUntil,
			OlderThan:  j.OlderThan,
			StartedAt:  j.StartedAt,
			Silent:     j.Silent,
		})
		counts.purges++
	}

	for _, l := range rec.ShortLinks {
		export.ShortLinks = append(export.ShortLinks, storage.ShortLink{
			ShortID:  l.ShortID,
			GuildID:  guildID,
			Original: l.Original,
			UserID:   l.UserID,
			Created:  l.Created,
			Clicks:   l.Clicks,
		})
		counts.links++
	}

	for userID, t := range rec.TaskList {
		if t.UserID == "" {
			t.UserID = userID
		}
		export.Tasks = append(export.Tasks, storage.Task{
			GuildID:    guildID,
			UserID:     t.UserID,
			MessageID:  t.MessageID,
			AssignedAt: t.AssignedAt,
			ExpiresAt:  t.ExpiresAt,
			Status:     t.Status,
		})
		counts.tasks++
	}

	for userID, until := range rec.TaskCooldowns {
		export.TaskCooldowns = append(export.TaskCooldowns, storage.TaskCooldown{
			GuildID: guildID,
			UserID:  userID,
			Until:   until,
		})
		counts.cooldowns++
	}

	if err := store.ImportGuild(export); err != nil {
		return err
	}

	// A legacy duration the current parser cannot read would silently fall back
	// to the default at read time. Check it through the real parser rather than
	// re-implementing one here, and say so now instead of leaving it to be
	// noticed as a behaviour change months later.
	if rec.TaskCooldownDuration != "" {
		if _, err := store.GetTaskCooldownDuration(guildID); err != nil {
			log.Warn().
				Str("guild_id", guildID).
				Str("value", rec.TaskCooldownDuration).
				Msg("task_cooldown_duration_unparsed")
		}
	}
	return nil
}
