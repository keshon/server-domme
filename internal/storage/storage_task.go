package storage

import (
	"fmt"
	"log"
	st "server-domme/internal/storagetypes"
	"time"
)

func (s *Storage) SetTaskRole(guildID, roleID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}
	if record.TaskRoles == nil {
		record.TaskRoles = map[string]string{}
	}

	record.TaskRoles[roleID] = "tasker"

	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetTaskRoles(guildID string) (map[string]string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}
	if record.TaskRoles == nil {
		return nil, fmt.Errorf("no roles set")
	}

	taskerRoles := make(map[string]string)
	for roleID, roleType := range record.TaskRoles {
		if roleType == "tasker" {
			taskerRoles[roleID] = roleType
		}
	}
	if len(taskerRoles) == 0 {
		return nil, fmt.Errorf("no tasker roles set")
	}
	return taskerRoles, nil
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
	s.ds.Add(guildID, record)
	return nil
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
		s.ds.Add(guildID, record)
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
	s.ds.Add(guildID, record)
	return nil
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
		s.ds.Add(guildID, record)
	}

	return nil
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
			s.ds.Add(guildID, record)
		}
	}

	return nil
}
