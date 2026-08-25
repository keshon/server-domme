// Package storage persists guild settings, the command log, purge jobs, short
// links and roleplay task state in an embedded write-ahead-logged datastore.
package storage

import (
	"fmt"
	"os"
	"time"

	"github.com/keshon/datastore"
	"github.com/rs/zerolog"
)

// commandHistoryLimit caps the per-guild command log, applied by trimming the
// oldest rows on append.
const commandHistoryLimit = 50

// Storage owns the database and the collections declared on it. Every
// collection and index must be registered before Open, so construction is the
// only place the schema is described.
type Storage struct {
	db  *datastore.DB
	log zerolog.Logger

	settings  *datastore.Collection[*GuildSettings]
	cmdLog    *datastore.Collection[*CommandLogEntry]
	purgeJobs *datastore.Collection[*PurgeJob]
	shortLink *datastore.Collection[*ShortLink]
	tasks     *datastore.Collection[*Task]
	cooldowns *datastore.Collection[*TaskCooldown]

	cmdLogByGuild    *datastore.Index[*CommandLogEntry]
	purgeJobsByGuild *datastore.Index[*PurgeJob]
	shortLinkByGuild *datastore.Index[*ShortLink]
	tasksByGuild     *datastore.Index[*Task]
	cooldownsByGuild *datastore.Index[*TaskCooldown]
}

// NewStorage opens the database in dir, creating it if needed. The directory is
// locked for the lifetime of the process: a second process opening the same dir
// fails with datastore.ErrLocked.
func NewStorage(dir string, log zerolog.Logger) (*Storage, error) {
	if err := rejectLegacyStorePath(dir); err != nil {
		return nil, err
	}
	db := datastore.New(datastore.Options{Dir: dir, Logger: &log})
	s := &Storage{db: db, log: log}

	s.settings = datastore.Register[*GuildSettings](db, "guild_settings")
	s.cmdLog = datastore.Register[*CommandLogEntry](db, "command_log")
	s.purgeJobs = datastore.Register[*PurgeJob](db, "purge_jobs")
	s.shortLink = datastore.Register[*ShortLink](db, "short_links")
	s.tasks = datastore.Register[*Task](db, "tasks")
	s.cooldowns = datastore.Register[*TaskCooldown](db, "task_cooldowns")

	s.cmdLogByGuild = datastore.AddIndex(s.cmdLog, "guild",
		func(c *CommandLogEntry) []string { return []string{c.GuildID} })
	s.purgeJobsByGuild = datastore.AddIndex(s.purgeJobs, "guild",
		func(p *PurgeJob) []string { return []string{p.GuildID} })
	s.shortLinkByGuild = datastore.AddIndex(s.shortLink, "guild",
		func(l *ShortLink) []string { return []string{l.GuildID} })
	s.tasksByGuild = datastore.AddIndex(s.tasks, "guild",
		func(t *Task) []string { return []string{t.GuildID} })
	s.cooldownsByGuild = datastore.AddIndex(s.cooldowns, "guild",
		func(c *TaskCooldown) []string { return []string{c.GuildID} })

	if err := db.Open(); err != nil {
		return nil, err
	}
	return s, nil
}

// rejectLegacyStorePath catches a STORAGE_PATH still pointing at the pre-v1
// single-file store.
//
// Without it the datastore fails with a bare "mkdir ./data/datastore.json: the
// system cannot find the path specified", which says nothing about the thing
// that actually changed — STORAGE_PATH names a directory now, not a file — and
// sends an upgrader looking for a missing path rather than a stale setting.
func rejectLegacyStorePath(dir string) error {
	info, err := os.Stat(dir)
	if err != nil || info.IsDir() {
		// Absent is fine: the datastore creates the directory it owns.
		return nil
	}
	return fmt.Errorf(
		"STORAGE_PATH %q is a file, but the store is a directory the datastore owns. "+
			"Convert the old one with: go run ./cmd/migrate-store -in %s -out ./data/store  "+
			"(then set STORAGE_PATH=./data/store)",
		dir, dir)
}

// Close compacts and releases the directory lock. Safe to call twice.
func (s *Storage) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// guildSettings returns the guild's settings, or a blank set for an unknown
// guild. The datastore decodes a fresh copy per read, so the result is private
// to this caller: mutate it and Put it back.
func (s *Storage) guildSettings(guildID string) *GuildSettings {
	if g, ok := s.settings.Get(guildID); ok {
		return g
	}
	return &GuildSettings{GuildID: guildID}
}

// trimOldest deletes the oldest rows so that appending one more leaves at most
// limit rows. existing must be in key order (which is chronological).
func trimOldest[T datastore.Entity](col *datastore.TxCollection[T], existing []T, limit int) error {
	over := len(existing) + 1 - limit
	for i := 0; i < over && i < len(existing); i++ {
		if err := col.Delete(existing[i].Key()); err != nil {
			return err
		}
	}
	return nil
}

// SetCommand records one command invocation, trimming the guild's oldest.
func (s *Storage) SetCommand(
	guildID, channelID, channelName, guildName, userID, username, command string,
) error {
	entry := &CommandLogEntry{
		GuildID:     guildID,
		ChannelID:   channelID,
		ChannelName: channelName,
		GuildName:   guildName,
		UserID:      userID,
		Username:    username,
		Command:     command,
		Datetime:    time.Now(),
	}
	return s.db.Update(func(tx *datastore.Tx) error {
		entry.ID = tx.NextID("cmdlog:" + guildID)
		col := datastore.In(tx, s.cmdLog)
		if err := col.Put(entry); err != nil {
			return err
		}
		// Read the index inside the transaction: the rows we trim against are
		// then the rows the commit sees, not a snapshot from before the writer
		// slot was ours. (The entry staged above is not indexed until commit,
		// so `existing` is the guild's rows without it — which is what the
		// limit arithmetic in trimOldest expects.)
		existing := datastore.InIndex(tx, s.cmdLogByGuild).Find(guildID)
		return trimOldest(col, existing, commandHistoryLimit)
	})
}

// CommandHistory returns the guild's recorded commands, oldest first.
func (s *Storage) CommandHistory(guildID string) ([]CommandLogEntry, error) {
	rows := s.cmdLogByGuild.Find(guildID)
	out := make([]CommandLogEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	return out, nil
}

// GuildExport is the shape served by the maintenance data dump.
type GuildExport struct {
	GuildID       string            `json:"guild_id"`
	Settings      GuildSettings     `json:"settings"`
	CommandLog    []CommandLogEntry `json:"command_log"`
	PurgeJobs     []PurgeJob        `json:"purge_jobs"`
	ShortLinks    []ShortLink       `json:"short_links"`
	Tasks         []Task            `json:"tasks"`
	TaskCooldowns []TaskCooldown    `json:"task_cooldowns"`
}

// ExportGuild gathers everything stored for one guild.
func (s *Storage) ExportGuild(guildID string) (GuildExport, error) {
	cmds, err := s.CommandHistory(guildID)
	if err != nil {
		return GuildExport{}, err
	}
	return GuildExport{
		GuildID:       guildID,
		Settings:      *s.guildSettings(guildID),
		CommandLog:    cmds,
		PurgeJobs:     deref(s.purgeJobsByGuild.Find(guildID)),
		ShortLinks:    deref(s.shortLinkByGuild.Find(guildID)),
		Tasks:         deref(s.tasksByGuild.Find(guildID)),
		TaskCooldowns: deref(s.cooldownsByGuild.Find(guildID)),
	}, nil
}

// deref turns an index read into value rows for the export shape. Index reads
// are already freshly decoded copies, so this is about the JSON the dump
// produces, not about isolating the caller.
func deref[T any](rows []*T) []T {
	out := make([]T, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	return out
}

// ImportGuild writes a GuildExport into the store verbatim, preserving the ids,
// timestamps and counters it carries.
//
// It exists for cmd/migrate-store and the running bot never calls it. The
// ordinary writers deliberately stamp their own values — SetCommand takes
// time.Now(), AddShortLink starts a link at zero clicks — which is right for a
// live invocation and wrong for a record being carried across from an older
// store, where doing so would rewrite every command's history to the moment of
// the migration.
//
// The whole guild lands in one transaction, so a failure part-way leaves
// nothing behind to clean up before retrying.
func (s *Storage) ImportGuild(e GuildExport) error {
	if e.GuildID == "" {
		return datastore.ErrEmptyKey
	}
	return s.db.Update(func(tx *datastore.Tx) error {
		settings := e.Settings
		settings.GuildID = e.GuildID
		if err := datastore.In(tx, s.settings).Put(&settings); err != nil {
			return err
		}

		// Ids are assigned fresh rather than carried: they only have to order
		// the rows within this guild, and the source order is what encodes that.
		// Trim to the newest before writing, so the ids stay dense.
		cmdCol := datastore.In(tx, s.cmdLog)
		rows := e.CommandLog
		if len(rows) > commandHistoryLimit {
			rows = rows[len(rows)-commandHistoryLimit:]
		}
		for i := range rows {
			row := rows[i]
			row.GuildID = e.GuildID
			row.ID = tx.NextID("cmdlog:" + e.GuildID)
			if err := cmdCol.Put(&row); err != nil {
				return err
			}
		}

		purgeCol := datastore.In(tx, s.purgeJobs)
		for i := range e.PurgeJobs {
			job := e.PurgeJobs[i]
			job.GuildID = e.GuildID
			if err := purgeCol.Put(&job); err != nil {
				return err
			}
		}

		linkCol := datastore.In(tx, s.shortLink)
		for i := range e.ShortLinks {
			link := e.ShortLinks[i]
			link.GuildID = e.GuildID
			if _, taken := linkCol.Get(link.ShortID); taken {
				return fmt.Errorf("short link %q already exists", link.ShortID)
			}
			if err := linkCol.Put(&link); err != nil {
				return err
			}
		}

		taskCol := datastore.In(tx, s.tasks)
		for i := range e.Tasks {
			task := e.Tasks[i]
			task.GuildID = e.GuildID
			if err := taskCol.Put(&task); err != nil {
				return err
			}
		}

		cooldownCol := datastore.In(tx, s.cooldowns)
		for i := range e.TaskCooldowns {
			cooldown := e.TaskCooldowns[i]
			cooldown.GuildID = e.GuildID
			if err := cooldownCol.Put(&cooldown); err != nil {
				return err
			}
		}
		return nil
	})
}
