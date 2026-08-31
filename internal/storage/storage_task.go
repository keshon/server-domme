package storage

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/keshon/datastore"
)

// DefaultTaskCooldownDuration applies to any guild that has not set its own.
const DefaultTaskCooldownDuration = 3 * time.Hour

var taskCooldownDurationPattern = regexp.MustCompile(`(?i)(\d+)([smhdw])`)

func parseTaskCooldownDuration(input string) (time.Duration, error) {
	matches := taskCooldownDurationPattern.FindAllStringSubmatch(input, -1)
	if matches == nil {
		return 0, errors.New("storage: invalid duration format")
	}

	var total time.Duration
	for _, match := range matches {
		value, _ := strconv.Atoi(match[1])
		switch match[2] {
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
			return 0, fmt.Errorf("storage: unknown time unit: %s", match[2])
		}
	}

	if total <= 0 {
		return 0, errors.New("storage: duration must be greater than zero")
	}
	return total, nil
}

// SetTaskRole sets the role whose members may draw tasks.
func (s *Storage) SetTaskRole(guildID, roleID string) error {
	g := s.guildSettings(guildID)
	g.TaskRole = roleID
	return s.settings.Put(g)
}

// GetTaskRole returns the guild's task role, or an error when unset.
func (s *Storage) GetTaskRole(guildID string) (string, error) {
	roleID := s.guildSettings(guildID).TaskRole
	if roleID == "" {
		return "", fmt.Errorf("storage: no tasker role set")
	}
	return roleID, nil
}

// SetTask records the task a member currently holds.
func (s *Storage) SetTask(guildID string, userID string, task Task) error {
	task.GuildID = guildID
	task.UserID = userID
	return s.tasks.Put(&task)
}

// GetTask returns the member's current task, or an error when they hold none.
func (s *Storage) GetTask(guildID string, userID string) (*Task, error) {
	task, ok := s.tasks.Get(guildScopedKey(guildID, userID))
	if !ok {
		return nil, fmt.Errorf("storage: no task for user %s", userID)
	}
	return task, nil
}

// ClearTask drops the member's current task (idempotent).
func (s *Storage) ClearTask(guildID string, userID string) error {
	return s.tasks.Delete(guildScopedKey(guildID, userID))
}

// SetCooldown blocks the member from drawing another task until cooldown.
func (s *Storage) SetCooldown(guildID string, userID string, cooldown time.Time) error {
	return s.cooldowns.Put(&TaskCooldown{
		GuildID: guildID,
		UserID:  userID,
		Until:   cooldown,
	})
}

// GetCooldown returns when the member may draw again, or an error when they are
// not on cooldown.
func (s *Storage) GetCooldown(guildID string, userID string) (time.Time, error) {
	c, ok := s.cooldowns.Get(guildScopedKey(guildID, userID))
	if !ok {
		return time.Time{}, fmt.Errorf("storage: no cooldown for user %s", userID)
	}
	return c.Until, nil
}

// ClearCooldown lifts a member's cooldown (idempotent).
func (s *Storage) ClearCooldown(guildID string, userID string) error {
	return s.cooldowns.Delete(guildScopedKey(guildID, userID))
}

// SetTaskCooldownDuration sets the guild's cooldown window, rejecting input the
// parser cannot read so a typo cannot silently fall back to the default.
func (s *Storage) SetTaskCooldownDuration(guildID, raw string) error {
	if _, err := parseTaskCooldownDuration(raw); err != nil {
		return err
	}
	g := s.guildSettings(guildID)
	g.TaskCooldownDuration = raw
	return s.settings.Put(g)
}

// GetTaskCooldownDuration returns the guild's cooldown window, or the default.
func (s *Storage) GetTaskCooldownDuration(guildID string) (time.Duration, error) {
	raw := s.guildSettings(guildID).TaskCooldownDuration
	if raw == "" {
		return DefaultTaskCooldownDuration, nil
	}
	return parseTaskCooldownDuration(raw)
}

// IsTaskCooldownDurationDefault reports whether the guild uses the default
// window.
func (s *Storage) IsTaskCooldownDurationDefault(guildID string) (bool, error) {
	return s.guildSettings(guildID).TaskCooldownDuration == "", nil
}

// ListActiveTaskCooldowns returns the guild's unexpired cooldowns by user id.
func (s *Storage) ListActiveTaskCooldowns(guildID string) (map[string]time.Time, error) {
	now := time.Now()
	active := make(map[string]time.Time)
	for _, c := range s.cooldownsByGuild.Find(guildID) {
		if c.Until.After(now) {
			active[c.UserID] = c.Until
		}
	}
	return active, nil
}

// ClearExpiredCooldowns deletes every elapsed cooldown across all guilds.
//
// Expiry is decided by comparing Until against now on read, so this only
// controls how long dead rows linger — never whether a cooldown is enforced.
func (s *Storage) ClearExpiredCooldowns() error {
	now := time.Now()
	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.cooldowns)
		expired := 0
		for c := range col.All() {
			if c.Until.Before(now) {
				if err := col.Delete(c.Key()); err != nil {
					return err
				}
				expired++
			}
		}
		if expired > 0 {
			s.log.Debug().Int("expired", expired).Msg("task_cooldowns_expired")
		}
		return nil
	})
}
