package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func newTestStore(t *testing.T) *Storage {
	t.Helper()
	s, err := NewStorage(t.TempDir(), zerolog.Nop())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func TestGuildSettingsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	if err := s.SetAnnounceChannel(guild, "chan-a"); err != nil {
		t.Fatalf("SetAnnounceChannel: %v", err)
	}
	if err := s.SetConfessChannel(guild, "chan-c"); err != nil {
		t.Fatalf("SetConfessChannel: %v", err)
	}
	if err := s.SetTaskRole(guild, "role-t"); err != nil {
		t.Fatalf("SetTaskRole: %v", err)
	}
	if err := s.SetPunishRole(guild, "punisher", "role-p"); err != nil {
		t.Fatalf("SetPunishRole: %v", err)
	}
	if err := s.SetPunishRole(guild, "victim", "role-v"); err != nil {
		t.Fatalf("SetPunishRole: %v", err)
	}

	// Each setter writes the whole settings row, so the risk worth testing is
	// that a later write drops an earlier field.
	if got, err := s.GetAnnounceChannel(guild); err != nil || got != "chan-a" {
		t.Errorf("GetAnnounceChannel = %q, %v; want chan-a", got, err)
	}
	if got, err := s.GetConfessChannel(guild); err != nil || got != "chan-c" {
		t.Errorf("GetConfessChannel = %q, %v; want chan-c", got, err)
	}
	if got, err := s.GetTaskRole(guild); err != nil || got != "role-t" {
		t.Errorf("GetTaskRole = %q, %v; want role-t", got, err)
	}
	if got, err := s.GetPunishRole(guild, "punisher"); err != nil || got != "role-p" {
		t.Errorf("GetPunishRole(punisher) = %q, %v; want role-p", got, err)
	}
	if got, err := s.GetPunishRole(guild, "victim"); err != nil || got != "role-v" {
		t.Errorf("GetPunishRole(victim) = %q, %v; want role-v", got, err)
	}
}

func TestGuildsAreIsolated(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetAnnounceChannel("g1", "chan-1"); err != nil {
		t.Fatalf("SetAnnounceChannel: %v", err)
	}
	if err := s.SetAnnounceChannel("g2", "chan-2"); err != nil {
		t.Fatalf("SetAnnounceChannel: %v", err)
	}
	if got, _ := s.GetAnnounceChannel("g1"); got != "chan-1" {
		t.Errorf("g1 announce = %q, want chan-1", got)
	}
	if got, _ := s.GetAnnounceChannel("g2"); got != "chan-2" {
		t.Errorf("g2 announce = %q, want chan-2", got)
	}
	if _, err := s.GetAnnounceChannel("g3"); err == nil {
		t.Error("unset guild returned no error")
	}
}

func TestCommandHistoryTrimsToLimit(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	total := commandHistoryLimit + 10
	for i := range total {
		if err := s.SetCommand(guild, "ch", "chan", "guild", "u", "user", cmdName(i)); err != nil {
			t.Fatalf("SetCommand %d: %v", i, err)
		}
	}

	rows, err := s.CommandHistory(guild)
	if err != nil {
		t.Fatalf("CommandHistory: %v", err)
	}
	if len(rows) != commandHistoryLimit {
		t.Fatalf("history len = %d, want %d", len(rows), commandHistoryLimit)
	}
	// Oldest-first ordering is what makes the trim keep the newest rows; if the
	// key padding ever stops sorting numerically this is the assertion that fails.
	if want := cmdName(total - commandHistoryLimit); rows[0].Command != want {
		t.Errorf("oldest kept = %q, want %q", rows[0].Command, want)
	}
	if want := cmdName(total - 1); rows[len(rows)-1].Command != want {
		t.Errorf("newest kept = %q, want %q", rows[len(rows)-1].Command, want)
	}
}

func cmdName(i int) string {
	return "cmd-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestCommandHistoryIsPerGuild(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetCommand("g1", "c", "chan", "guild", "u", "user", "only-g1"); err != nil {
		t.Fatalf("SetCommand: %v", err)
	}
	rows, _ := s.CommandHistory("g2")
	if len(rows) != 0 {
		t.Fatalf("g2 history len = %d, want 0", len(rows))
	}
}

func TestTaskLifecycle(t *testing.T) {
	s := newTestStore(t)
	const guild, user = "g1", "u1"

	if _, err := s.GetTask(guild, user); err == nil {
		t.Error("GetTask on empty store returned no error")
	}

	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := s.SetTask(guild, user, Task{MessageID: "m1", ExpiresAt: expires, Status: "pending"}); err != nil {
		t.Fatalf("SetTask: %v", err)
	}

	got, err := s.GetTask(guild, user)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.MessageID != "m1" || got.Status != "pending" {
		t.Errorf("task = %+v, want MessageID m1 / status pending", got)
	}
	// SetTask fills these from its arguments; a caller that reads them back
	// expects the row to know which guild and member it belongs to.
	if got.GuildID != guild || got.UserID != user {
		t.Errorf("task identity = %q/%q, want %q/%q", got.GuildID, got.UserID, guild, user)
	}

	if err := s.ClearTask(guild, user); err != nil {
		t.Fatalf("ClearTask: %v", err)
	}
	if _, err := s.GetTask(guild, user); err == nil {
		t.Error("GetTask after ClearTask returned no error")
	}
	if err := s.ClearTask(guild, user); err != nil {
		t.Errorf("ClearTask is not idempotent: %v", err)
	}
}

// TestSetCooldownSurvivesExpirySweep covers the bug the old whole-guild-blob
// model had: SetCooldown ran the expiry sweep, which wrote the same record
// independently, and the write that followed discarded it.
func TestSetCooldownSurvivesExpirySweep(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	if err := s.SetCooldown(guild, "stale", past); err != nil {
		t.Fatalf("SetCooldown(stale): %v", err)
	}
	if err := s.SetCooldown(guild, "fresh", future); err != nil {
		t.Fatalf("SetCooldown(fresh): %v", err)
	}
	if err := s.ClearExpiredCooldowns(); err != nil {
		t.Fatalf("ClearExpiredCooldowns: %v", err)
	}

	if _, err := s.GetCooldown(guild, "stale"); err == nil {
		t.Error("expired cooldown survived the sweep")
	}
	if got, err := s.GetCooldown(guild, "fresh"); err != nil {
		t.Errorf("live cooldown was swept: %v", err)
	} else if !got.Equal(future) {
		t.Errorf("cooldown = %v, want %v", got, future)
	}

	active, err := s.ListActiveTaskCooldowns(guild)
	if err != nil {
		t.Fatalf("ListActiveTaskCooldowns: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active cooldowns = %d, want 1", len(active))
	}
}

func TestTaskCooldownDuration(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	if d, err := s.GetTaskCooldownDuration(guild); err != nil || d != DefaultTaskCooldownDuration {
		t.Errorf("unset duration = %v, %v; want %v", d, err, DefaultTaskCooldownDuration)
	}
	if isDefault, _ := s.IsTaskCooldownDurationDefault(guild); !isDefault {
		t.Error("unset duration not reported as default")
	}

	if err := s.SetTaskCooldownDuration(guild, "1h30m"); err != nil {
		t.Fatalf("SetTaskCooldownDuration: %v", err)
	}
	if d, err := s.GetTaskCooldownDuration(guild); err != nil || d != 90*time.Minute {
		t.Errorf("duration = %v, %v; want 1h30m", d, err)
	}
	if isDefault, _ := s.IsTaskCooldownDurationDefault(guild); isDefault {
		t.Error("set duration still reported as default")
	}

	// A rejected value must not be stored: silently falling back to the default
	// would make a typo look like it applied.
	if err := s.SetTaskCooldownDuration(guild, "banana"); err == nil {
		t.Error("invalid duration accepted")
	}
	if d, _ := s.GetTaskCooldownDuration(guild); d != 90*time.Minute {
		t.Errorf("duration after rejected write = %v, want 1h30m", d)
	}
}

func TestShortLinkIDsAreGlobal(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddShortLink("g1", "u1", "https://example.com/a", "abc"); err != nil {
		t.Fatalf("AddShortLink: %v", err)
	}
	// The redirect server resolves by id alone, so a second guild reusing an id
	// would silently repoint the first guild's link.
	if err := s.AddShortLink("g2", "u2", "https://example.com/b", "abc"); err == nil {
		t.Error("duplicate short id across guilds was accepted")
	}

	guildID, link, err := s.FindLinkByID("abc")
	if err != nil {
		t.Fatalf("FindLinkByID: %v", err)
	}
	if guildID != "g1" || link.Original != "https://example.com/a" {
		t.Errorf("resolved to %q/%q, want g1/https://example.com/a", guildID, link.Original)
	}
}

func TestShortLinkClicksAndDeletion(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddShortLink("g1", "u1", "https://example.com/a", "abc"); err != nil {
		t.Fatalf("AddShortLink: %v", err)
	}
	for range 3 {
		if err := s.IncrementClicks("g1", "abc"); err != nil {
			t.Fatalf("IncrementClicks: %v", err)
		}
	}
	_, link, err := s.FindLinkByID("abc")
	if err != nil {
		t.Fatalf("FindLinkByID: %v", err)
	}
	if link.Clicks != 3 {
		t.Errorf("clicks = %d, want 3", link.Clicks)
	}

	// Deleting is owner-scoped: the id addresses every link in the store, so a
	// bare lookup would let one member delete another's.
	if err := s.DeleteShortLink("g1", "someone-else", "abc"); err == nil {
		t.Error("delete succeeded for a non-owner")
	}
	if err := s.DeleteShortLink("g1", "u1", "abc"); err != nil {
		t.Fatalf("DeleteShortLink: %v", err)
	}
	if _, _, err := s.FindLinkByID("abc"); err == nil {
		t.Error("link survived deletion")
	}
}

func TestClearUserShortLinks(t *testing.T) {
	s := newTestStore(t)

	for _, id := range []string{"a1", "a2"} {
		if err := s.AddShortLink("g1", "u1", "https://example.com/"+id, id); err != nil {
			t.Fatalf("AddShortLink: %v", err)
		}
	}
	if err := s.AddShortLink("g1", "u2", "https://example.com/keep", "keep"); err != nil {
		t.Fatalf("AddShortLink: %v", err)
	}

	if err := s.ClearUserShortLinks("g1", "u1"); err != nil {
		t.Fatalf("ClearUserShortLinks: %v", err)
	}
	if links, _ := s.GetUserShortLinks("g1", "u1"); len(links) != 0 {
		t.Errorf("u1 links = %d, want 0", len(links))
	}
	if links, _ := s.GetUserShortLinks("g1", "u2"); len(links) != 1 {
		t.Errorf("u2 links = %d, want 1", len(links))
	}
}

func TestPurgeJobs(t *testing.T) {
	s := newTestStore(t)

	until := time.Now().Add(time.Hour)
	if err := s.SetDeletionJob("g1", "c1", "delayed", until, false); err != nil {
		t.Fatalf("SetDeletionJob: %v", err)
	}
	if err := s.SetDeletionJob("g1", "c2", "recurring", time.Time{}, true, "24h"); err != nil {
		t.Fatalf("SetDeletionJob: %v", err)
	}
	if err := s.SetDeletionJob("g2", "c3", "delayed", until, false); err != nil {
		t.Fatalf("SetDeletionJob: %v", err)
	}

	jobs, err := s.GetDeletionJobsList("g1")
	if err != nil {
		t.Fatalf("GetDeletionJobsList: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("g1 jobs = %d, want 2", len(jobs))
	}
	if jobs["c2"].OlderThan != "24h" {
		t.Errorf("c2 OlderThan = %q, want 24h", jobs["c2"].OlderThan)
	}

	// A channel with no job reads as the zero job, which is what callers test.
	if job, err := s.GetDeletionJob("g1", "nope"); err != nil || job.Mode != "" {
		t.Errorf("missing job = %+v, %v; want zero job and no error", job, err)
	}

	if all := s.AllPurgeJobs(); len(all) != 3 {
		t.Errorf("AllPurgeJobs = %d, want 3", len(all))
	}

	if err := s.ClearDeletionJob("g1", "c1"); err != nil {
		t.Fatalf("ClearDeletionJob: %v", err)
	}
	if jobs, _ := s.GetDeletionJobsList("g1"); len(jobs) != 1 {
		t.Errorf("g1 jobs after clear = %d, want 1", len(jobs))
	}
}

func TestCommandGroupToggles(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	if disabled, _ := s.IsGroupDisabled(guild, "task"); disabled {
		t.Error("group disabled before any write")
	}
	if err := s.DisableGroup(guild, "task"); err != nil {
		t.Fatalf("DisableGroup: %v", err)
	}
	if err := s.DisableGroup(guild, "task"); err != nil {
		t.Fatalf("DisableGroup is not idempotent: %v", err)
	}
	if groups, _ := s.DisabledGroups(guild); len(groups) != 1 {
		t.Errorf("disabled groups = %v, want exactly one", groups)
	}
	if disabled, _ := s.IsGroupDisabled(guild, "task"); !disabled {
		t.Error("group not reported disabled")
	}

	if err := s.EnableGroup(guild, "task"); err != nil {
		t.Fatalf("EnableGroup: %v", err)
	}
	if disabled, _ := s.IsGroupDisabled(guild, "task"); disabled {
		t.Error("group still disabled after EnableGroup")
	}
}

func TestMediaDefaultClearedWithItsCategory(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	if err := s.CreateMediaCategory(guild, "cats"); err != nil {
		t.Fatalf("CreateMediaCategory: %v", err)
	}
	if err := s.SetMediaDefault(guild, "cats"); err != nil {
		t.Fatalf("SetMediaDefault: %v", err)
	}
	if err := s.RemoveMediaCategory(guild, "cats"); err != nil {
		t.Fatalf("RemoveMediaCategory: %v", err)
	}

	// A default pointing at a removed category would send /media looking in a
	// directory nothing writes to.
	if def, _ := s.GetMediaDefault(guild); def != "" {
		t.Errorf("media default = %q after its category was removed, want empty", def)
	}
	if cats, _ := s.GetMediaCategories(guild); len(cats) != 0 {
		t.Errorf("categories = %v, want none", cats)
	}
}

func TestTranslateChannels(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	if err := s.AddTranslateChannel(guild, "c1"); err != nil {
		t.Fatalf("AddTranslateChannel: %v", err)
	}
	if err := s.AddTranslateChannel(guild, "c1"); err == nil {
		t.Error("duplicate translate channel accepted")
	}
	if err := s.RemoveTranslateChannel(guild, "nope"); err == nil {
		t.Error("removing an absent channel reported success")
	}
	if err := s.RemoveTranslateChannel(guild, "c1"); err != nil {
		t.Fatalf("RemoveTranslateChannel: %v", err)
	}
	if channels, _ := s.GetTranslateChannels(guild); len(channels) != 0 {
		t.Errorf("channels = %v, want none", channels)
	}
}

func TestDataSurvivesReopen(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStorage(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if err := s.SetAnnounceChannel("g1", "chan-a"); err != nil {
		t.Fatalf("SetAnnounceChannel: %v", err)
	}
	if err := s.SetCommand("g1", "c", "chan", "guild", "u", "user", "about"); err != nil {
		t.Fatalf("SetCommand: %v", err)
	}
	if err := s.AddShortLink("g1", "u1", "https://example.com/a", "abc"); err != nil {
		t.Fatalf("AddShortLink: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := NewStorage(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	if got, err := reopened.GetAnnounceChannel("g1"); err != nil || got != "chan-a" {
		t.Errorf("announce after reopen = %q, %v; want chan-a", got, err)
	}
	if rows, _ := reopened.CommandHistory("g1"); len(rows) != 1 || rows[0].Command != "about" {
		t.Errorf("command log after reopen = %+v, want one 'about' row", rows)
	}
	if _, link, err := reopened.FindLinkByID("abc"); err != nil || link.Original != "https://example.com/a" {
		t.Errorf("short link after reopen = %+v, %v", link, err)
	}
}

func TestExportGuild(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	if err := s.SetAnnounceChannel(guild, "chan-a"); err != nil {
		t.Fatalf("SetAnnounceChannel: %v", err)
	}
	if err := s.SetCommand(guild, "c", "chan", "guild", "u", "user", "about"); err != nil {
		t.Fatalf("SetCommand: %v", err)
	}
	if err := s.AddShortLink(guild, "u1", "https://example.com/a", "abc"); err != nil {
		t.Fatalf("AddShortLink: %v", err)
	}
	if err := s.SetCommand("other", "c", "chan", "guild", "u", "user", "about"); err != nil {
		t.Fatalf("SetCommand(other): %v", err)
	}

	export, err := s.ExportGuild(guild)
	if err != nil {
		t.Fatalf("ExportGuild: %v", err)
	}
	if export.GuildID != guild {
		t.Errorf("GuildID = %q, want %q", export.GuildID, guild)
	}
	if export.Settings.AnnounceChannel != "chan-a" {
		t.Errorf("settings not exported: %+v", export.Settings)
	}
	// The dump is handed to one guild's admins, so it must not carry another
	// guild's rows.
	if len(export.CommandLog) != 1 {
		t.Errorf("command log = %d rows, want 1 (other guild leaked in?)", len(export.CommandLog))
	}
	if len(export.ShortLinks) != 1 {
		t.Errorf("short links = %d, want 1", len(export.ShortLinks))
	}
}

// TestImportGuildPreservesVerbatim is the property the migrator depends on and
// the one the ordinary setters break: SetCommand stamps time.Now() and
// AddShortLink starts a link at zero clicks, so a migration routed through them
// rewrites every record's history to the moment it ran.
func TestImportGuildPreservesVerbatim(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	oldTime := time.Date(2021, 3, 4, 5, 6, 7, 8, time.UTC)
	in := GuildExport{
		GuildID: guild,
		Settings: GuildSettings{
			AnnounceChannel:      "chan-a",
			TaskCooldownDuration: "3d",
			MediaCategories:      []string{"cats", "dogs"},
		},
		CommandLog: []CommandLogEntry{
			{ChannelID: "c", Command: "older", Datetime: oldTime},
			{ChannelID: "c", Command: "newer", Datetime: oldTime.Add(time.Hour)},
		},
		ShortLinks: []ShortLink{
			{ShortID: "abc", Original: "https://example.com/a", UserID: "u1", Created: oldTime, Clicks: 42},
		},
		Tasks: []Task{
			{UserID: "u1", MessageID: "m1", AssignedAt: oldTime, ExpiresAt: oldTime.Add(time.Hour), Status: "pending"},
		},
		TaskCooldowns: []TaskCooldown{
			{UserID: "u2", Until: oldTime.Add(24 * time.Hour)},
		},
		PurgeJobs: []PurgeJob{
			{ChannelID: "c1", Mode: "recurring", OlderThan: "24h", StartedAt: oldTime, Silent: true},
		},
	}

	if err := s.ImportGuild(in); err != nil {
		t.Fatalf("ImportGuild: %v", err)
	}

	out, err := s.ExportGuild(guild)
	if err != nil {
		t.Fatalf("ExportGuild: %v", err)
	}

	if out.Settings.AnnounceChannel != "chan-a" || out.Settings.TaskCooldownDuration != "3d" {
		t.Errorf("settings = %+v", out.Settings)
	}
	if len(out.CommandLog) != 2 {
		t.Fatalf("command log = %d rows, want 2", len(out.CommandLog))
	}
	// Order has to survive too: ids are assigned on import, and they are what
	// the oldest-first read depends on.
	if out.CommandLog[0].Command != "older" || out.CommandLog[1].Command != "newer" {
		t.Errorf("command order = %q, %q", out.CommandLog[0].Command, out.CommandLog[1].Command)
	}
	for i, want := range in.CommandLog {
		if !out.CommandLog[i].Datetime.Equal(want.Datetime) {
			t.Errorf("command %d datetime = %v, want %v", i, out.CommandLog[i].Datetime, want.Datetime)
		}
	}

	if len(out.ShortLinks) != 1 {
		t.Fatalf("short links = %d, want 1", len(out.ShortLinks))
	}
	if got := out.ShortLinks[0]; got.Clicks != 42 || !got.Created.Equal(oldTime) {
		t.Errorf("short link = clicks %d, created %v; want 42, %v", got.Clicks, got.Created, oldTime)
	}
	if len(out.Tasks) != 1 || !out.Tasks[0].AssignedAt.Equal(oldTime) {
		t.Errorf("tasks = %+v", out.Tasks)
	}
	if len(out.TaskCooldowns) != 1 || !out.TaskCooldowns[0].Until.Equal(oldTime.Add(24*time.Hour)) {
		t.Errorf("cooldowns = %+v", out.TaskCooldowns)
	}
	if len(out.PurgeJobs) != 1 || out.PurgeJobs[0].OlderThan != "24h" || !out.PurgeJobs[0].Silent {
		t.Errorf("purge jobs = %+v", out.PurgeJobs)
	}
}

func TestImportGuildRejectsEmptyGuildID(t *testing.T) {
	s := newTestStore(t)
	// An empty guild id cannot be stored: the settings key would be empty, and
	// log rows would be left out of the by-guild index and become unreadable.
	if err := s.ImportGuild(GuildExport{GuildID: ""}); err == nil {
		t.Error("ImportGuild accepted an empty guild id")
	}
}

func TestImportGuildTrimsCommandLog(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	in := GuildExport{GuildID: guild}
	total := commandHistoryLimit + 5
	base := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range total {
		in.CommandLog = append(in.CommandLog, CommandLogEntry{
			Command:  cmdName(i),
			Datetime: base.Add(time.Duration(i) * time.Minute),
		})
	}
	if err := s.ImportGuild(in); err != nil {
		t.Fatalf("ImportGuild: %v", err)
	}

	out, _ := s.ExportGuild(guild)
	if len(out.CommandLog) != commandHistoryLimit {
		t.Fatalf("command log = %d, want %d", len(out.CommandLog), commandHistoryLimit)
	}
	// The newest are what survive, matching what the live writer would keep.
	if want := cmdName(total - 1); out.CommandLog[len(out.CommandLog)-1].Command != want {
		t.Errorf("newest = %q, want %q", out.CommandLog[len(out.CommandLog)-1].Command, want)
	}
	if want := cmdName(total - commandHistoryLimit); out.CommandLog[0].Command != want {
		t.Errorf("oldest kept = %q, want %q", out.CommandLog[0].Command, want)
	}
}

// TestNewStorageRejectsLegacyFilePath covers the upgrade trap: STORAGE_PATH used
// to name a single JSON file and now names a directory. Pointing the new build
// at the old value must say so, not fail with a bare mkdir error.
func TestNewStorageRejectsLegacyFilePath(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "datastore.json")
	if err := os.WriteFile(legacy, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}

	_, err := NewStorage(legacy, zerolog.Nop())
	if err == nil {
		t.Fatal("NewStorage accepted a file path")
	}
	// The message has to name the fix, or it is no better than the mkdir error.
	for _, want := range []string{"STORAGE_PATH", "directory", "migrate-store"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	// The legacy file must survive being pointed at.
	if _, statErr := os.Stat(legacy); statErr != nil {
		t.Errorf("legacy file was disturbed: %v", statErr)
	}
}

func TestNewStorageAcceptsMissingAndExistingDir(t *testing.T) {
	base := t.TempDir()

	fresh := filepath.Join(base, "does-not-exist-yet")
	s, err := NewStorage(fresh, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewStorage on a missing dir: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := NewStorage(fresh, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewStorage on an existing dir: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
