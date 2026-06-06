package storage

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	st "github.com/keshon/server-domme/internal/domain"
)

const DefaultTaskCooldownDuration = 3 * time.Hour

var taskCooldownDurationPattern = regexp.MustCompile(`(?i)(\d+)([smhdw])`)

func parseTaskCooldownDuration(input string) (time.Duration, error) {
	matches := taskCooldownDurationPattern.FindAllStringSubmatch(input, -1)
	if matches == nil {
		return 0, errors.New("invalid duration format")
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
			return 0, fmt.Errorf("unknown time unit: %s", match[2])
		}
	}

	if total <= 0 {
		return 0, errors.New("duration must be greater than zero")
	}
	return total, nil
}

func (s *Storage) SetTaskRole(guildID, roleID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.TaskRole = roleID
	return s.ds.Set(guildID, record)
}

func (s *Storage) GetTaskRole(guildID string) (string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return "", err
	}

	if record.TaskRole == "" {
		return "", fmt.Errorf("no tasker role set")
	}
	return record.TaskRole, nil
}

func (s *Storage) SetTask(guildID string, userID string, task st.Task) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if record.TaskList == nil {
		record.TaskList = make(map[string]st.Task)
	}

	record.TaskList[userID] = task
	return s.ds.Set(guildID, record)
}

func (s *Storage) GetTask(guildID string, userID string) (*st.Task, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}

	if record.TaskList == nil {
		return nil, fmt.Errorf("no tasks found")
	}

	task, exists := record.TaskList[userID]
	if !exists {
		return nil, fmt.Errorf("no task for user %s", userID)
	}

	return &task, nil
}

func (s *Storage) ClearTask(guildID string, userID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if record.TaskList != nil {
		delete(record.TaskList, userID)
		return s.ds.Set(guildID, record)
	}

	return nil
}

func (s *Storage) SetCooldown(guildID string, userID string, cooldown time.Time) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	s.ClearExpiredCooldowns()

	if record.TaskCooldowns == nil {
		record.TaskCooldowns = make(map[string]time.Time)
	}

	record.TaskCooldowns[userID] = cooldown
	return s.ds.Set(guildID, record)
}

func (s *Storage) GetCooldown(guildID string, userID string) (time.Time, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return time.Time{}, err
	}

	if record.TaskCooldowns == nil {
		return time.Time{}, fmt.Errorf("no cooldown found")
	}

	cooldown, exists := record.TaskCooldowns[userID]
	if !exists {
		return time.Time{}, fmt.Errorf("no cooldown for user %s", userID)
	}

	return cooldown, nil
}

func (s *Storage) ClearCooldown(guildID string, userID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if record.TaskCooldowns != nil {
		delete(record.TaskCooldowns, userID)
		return s.ds.Set(guildID, record)
	}

	return nil
}

func (s *Storage) SetTaskCooldownDuration(guildID, raw string) error {
	if _, err := parseTaskCooldownDuration(raw); err != nil {
		return err
	}

	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.TaskCooldownDuration = raw
	return s.ds.Set(guildID, record)
}

func (s *Storage) GetTaskCooldownDuration(guildID string) (time.Duration, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return DefaultTaskCooldownDuration, err
	}

	if record.TaskCooldownDuration == "" {
		return DefaultTaskCooldownDuration, nil
	}

	return parseTaskCooldownDuration(record.TaskCooldownDuration)
}

func (s *Storage) IsTaskCooldownDurationDefault(guildID string) (bool, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return true, err
	}
	return record.TaskCooldownDuration == "", nil
}

func (s *Storage) ListActiveTaskCooldowns(guildID string) (map[string]time.Time, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}

	if record.TaskCooldowns == nil {
		return map[string]time.Time{}, nil
	}

	now := time.Now()
	active := make(map[string]time.Time)
	for userID, expiry := range record.TaskCooldowns {
		if expiry.After(now) {
			active[userID] = expiry
		}
	}
	return active, nil
}

func (s *Storage) ClearExpiredCooldowns() error {
	now := time.Now()

	for _, guildID := range s.ds.Keys() {
		record, err := s.getOrCreateGuildRecord(guildID)
		if err != nil {
			return fmt.Errorf("error fetching record for guild %s: %w", guildID, err)
		}

		if record.TaskCooldowns == nil {
			continue
		}

		changed := false
		for userID, cooldown := range record.TaskCooldowns {
			if cooldown.Before(now) {
				delete(record.TaskCooldowns, userID)
				changed = true
				log.Println("Expired cooldown for user", userID, "in guild", guildID)
			}
		}

		if changed {
			if err := s.ds.Set(guildID, record); err != nil {
				return err
			}
		}
	}

	return nil
}
