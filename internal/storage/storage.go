// /internal/storage/storage.go
package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"server-domme/datastore"
	st "server-domme/internal/storagetypes"
)

const commandHistoryLimit int = 50

type Storage struct {
	ds *datastore.DataStore
}

func New(filePath string) (*Storage, error) {
	ds, err := datastore.New(filePath)
	if err != nil {
		return nil, err
	}
	return &Storage{ds: ds}, nil
}

func (s *Storage) Close() error {
	return s.ds.Close()
}

func (s *Storage) getOrCreateGuildRecord(guildID string) (*st.Record, error) {
	data, exists := s.ds.Get(guildID)
	if !exists {
		newRecord := &st.Record{
			CommandsHistory: []st.CommandHistory{},
			Roles:           map[string]string{},
		}
		s.ds.Add(guildID, newRecord)
		return newRecord, nil
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error marshalling data: %w", err)
	}

	var record st.Record
	err = json.Unmarshal(jsonData, &record)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling to *Record: %w", err)
	}

	if record.Roles == nil {
		record.Roles = map[string]string{}
	}
	if record.Tasks == nil {
		record.Tasks = make(map[string]st.Task)
	}

	if record.DeletionJobs == nil {
		record.DeletionJobs = make(map[string]st.DeletionJob)
	}

	if len(record.CommandsHistory) > commandHistoryLimit {
		record.CommandsHistory = record.CommandsHistory[len(record.CommandsHistory)-commandHistoryLimit:]
	}

	return &record, nil
}

func (s *Storage) GetRecordsList() map[string]st.Record {
	mapStringAny := s.ds.GetAll()

	mapStringRecord := make(map[string]st.Record)
	for key, value := range mapStringAny {
		jsonData, err := json.Marshal(value)
		if err != nil {
			log.Printf("error marshalling data: %v", err)
			continue
		}

		var record st.Record
		err = json.Unmarshal(jsonData, &record)
		if err != nil {
			log.Printf("error unmarshalling to *Record: %v", err)
			continue
		}

		mapStringRecord[key] = record
	}
	return mapStringRecord
}

func (s *Storage) appendCommandToHistory(guildID string, command st.CommandHistory) error {

	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.CommandsHistory = append(record.CommandsHistory, command)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) SetCommand(
	guildID, channelID, channelName, guildName, userID, username, command string,
) error {
	record := st.CommandHistory{
		ChannelID:   channelID,
		ChannelName: channelName,
		GuildName:   guildName,
		UserID:      userID,
		Username:    username,
		Command:     command,
		Datetime:    time.Now(),
	}
	return s.appendCommandToHistory(guildID, record)
}

func (s *Storage) GetCommands(guildID string) ([]st.CommandHistory, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}

	return record.CommandsHistory, nil
}

func (s *Storage) SetPunishRole(guildID string, roleType string, roleID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if record.Roles == nil {
		record.Roles = map[string]string{}
	}

	record.Roles[roleType] = roleID
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetPunishRole(guildID string, roleType string) (string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return "", err
	}

	roleID, exists := record.Roles[roleType]
	if !exists {
		return "", fmt.Errorf("role type '%s' not set for this guild", roleType)
	}

	return roleID, nil
}

func (s *Storage) SetTaskRole(guildID, roleID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}
	if record.Roles == nil {
		record.Roles = map[string]string{}
	}

	record.Roles[roleID] = "tasker"

	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetTaskRoles(guildID string) (map[string]string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}
	if record.Roles == nil {
		return nil, fmt.Errorf("no roles set")
	}

	taskerRoles := make(map[string]string)
	for roleID, roleType := range record.Roles {
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

	if record.Tasks == nil {
		record.Tasks = make(map[string]st.Task)
	}

	record.Tasks[userID] = task
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetTask(guildID string, userID string) (*st.Task, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}

	if record.Tasks == nil {
		return nil, fmt.Errorf("no tasks found")
	}

	task, exists := record.Tasks[userID]
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

	if record.Tasks != nil {
		delete(record.Tasks, userID)
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

	if record.Cooldowns == nil {
		record.Cooldowns = make(map[string]time.Time)
	}

	record.Cooldowns[userID] = cooldown
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetCooldown(guildID string, userID string) (time.Time, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return time.Time{}, err
	}

	if record.Cooldowns == nil {
		return time.Time{}, fmt.Errorf("no cooldown found")
	}

	cooldown, exists := record.Cooldowns[userID]
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

	if record.Cooldowns != nil {
		delete(record.Cooldowns, userID)
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

		if record.Cooldowns == nil {
			continue
		}

		changed := false
		for userID, cooldown := range record.Cooldowns {
			if cooldown.Before(now) {
				delete(record.Cooldowns, userID)
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

func (s *Storage) SetDeletionJob(guildID, channelID, mode string, delayUntil time.Time, silent bool, olderThan ...string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	job := st.DeletionJob{
		ChannelID:  channelID,
		GuildID:    guildID,
		Mode:       mode,
		DelayUntil: delayUntil,
		Silent:     silent,
		StartedAt:  time.Now(),
	}

	if len(olderThan) > 0 {
		job.OlderThan = olderThan[0]
	}

	record.DeletionJobs[channelID] = job
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) ClearDeletionJob(guildID, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}
	delete(record.DeletionJobs, channelID)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetDeletionJobsList(guildID string) (map[string]st.DeletionJob, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}
	return record.DeletionJobs, nil
}

func (s *Storage) GetDeletionJob(guildID, channelID string) (st.DeletionJob, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return st.DeletionJob{}, err
	}
	return record.DeletionJobs[channelID], nil
}

func (s *Storage) GetMap(key string) (map[string]string, error) {
	raw, exists := s.ds.Get(key)
	if !exists {
		return map[string]string{}, nil
	}

	jsonData, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal raw data: %w", err)
	}

	var result map[string]string
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into map[string]string: %w", err)
	}

	return result, nil
}

func (s *Storage) SetMap(key string, value map[string]string) error {
	s.ds.Add(key, value)
	return nil
}

func (s *Storage) Dump() (map[string]interface{}, error) {
	all := make(map[string]interface{})
	for _, key := range s.ds.Keys() {
		value, exists := s.ds.Get(key)
		if !exists {
			continue
		}
		all[key] = value
	}
	return all, nil
}

func (s *Storage) SetSpecialChannel(guildID, kind, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if record.Channels == nil {
		record.Channels = map[string]string{}
	}

	record.Channels["channel_"+kind] = channelID
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetSpecialChannel(guildID, kind string) (string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return "", err
	}

	id, ok := record.Channels["channel_"+kind]
	if !ok || id == "" {
		return "", fmt.Errorf("channel not set for kind '%s'", kind)
	}
	return id, nil
}

func (s *Storage) DisableGroup(guildID, group string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	for _, g := range record.DisabledGroups {
		if g == group {
			return nil // уже выключено
		}
	}

	record.DisabledGroups = append(record.DisabledGroups, group)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) EnableGroup(guildID, group string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	updated := make([]string, 0, len(record.DisabledGroups))
	for _, g := range record.DisabledGroups {
		if g != group {
			updated = append(updated, g)
		}
	}
	record.DisabledGroups = updated
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) IsGroupDisabled(guildID, group string) (bool, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return false, err
	}
	for _, g := range record.DisabledGroups {
		if g == group {
			return true, nil
		}
	}
	return false, nil
}

func (s *Storage) GetDisabledGroups(guildID string) ([]string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}
	return record.DisabledGroups, nil
}
