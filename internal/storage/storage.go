// /internal/storage/storage.go
package storage

import (
	"encoding/json"
	"fmt"
	"time"

	"server-domme/datastore"
)

const (
	commandHistoryLimit int = 20
	tracksHistoryLimit  int = 12
)

type Storage struct {
	ds *datastore.DataStore
}

type CommandHistoryRecord struct {
	ChannelID   string    `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	GuildName   string    `json:"guild_name"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Command     string    `json:"command"`
	Param       string    `json:"param"`
	Datetime    time.Time `json:"datetime"`
}

type UserTask struct {
	UserID     string    `json:"user_id"`
	TaskText   string    `json:"task_text"`
	AssignedAt time.Time `json:"assigned_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Status     string    `json:"status"` // "pending", "completed", "failed", "safeword"
}

type Record struct {
	CommandsHistoryList []CommandHistoryRecord `json:"cmd_history"`
	Roles               map[string]string      `json:"roles"` // e.g., "punisher": "roleID"
	Tasks               map[string]UserTask    `json:"tasks"` // key = userID
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

// Helper function to get or create a Record for a guild
func (s *Storage) getOrCreateGuildRecord(guildID string) (*Record, error) {
	data, exists := s.ds.Get(guildID)
	if !exists {
		newRecord := &Record{
			CommandsHistoryList: []CommandHistoryRecord{},
			Roles:               map[string]string{},
		}
		s.ds.Add(guildID, newRecord)
		return newRecord, nil
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error marshalling data: %w", err)
	}

	var record Record
	err = json.Unmarshal(jsonData, &record)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling to *Record: %w", err)
	}

	if record.Roles == nil {
		record.Roles = map[string]string{}
	}
	if record.Tasks == nil {
		record.Tasks = make(map[string]UserTask)
	}

	if len(record.CommandsHistoryList) > commandHistoryLimit {
		record.CommandsHistoryList = record.CommandsHistoryList[len(record.CommandsHistoryList)-commandHistoryLimit:]
	}

	return &record, nil
}

// AppendCommandToHistory appends a command history record for a guild
func (s *Storage) AppendCommandToHistory(guildID string, command CommandHistoryRecord) error {

	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.CommandsHistoryList = append(record.CommandsHistoryList, command)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) FetchCommandHistory(guildID string) ([]CommandHistoryRecord, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}

	return record.CommandsHistoryList, nil
}

func (s *Storage) SetRoleForGuild(guildID string, roleType string, roleID string) error {
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

func (s *Storage) GetRoleForGuild(guildID string, roleType string) (string, error) {
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

func (s *Storage) SetUserTask(guildID string, userID string, task UserTask) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if record.Tasks == nil {
		record.Tasks = make(map[string]UserTask)
	}

	record.Tasks[userID] = task
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetUserTask(guildID string, userID string) (*UserTask, error) {
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

func (s *Storage) ClearUserTask(guildID string, userID string) error {
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
