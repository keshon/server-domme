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

type Record struct {
	CommandsHistoryList []CommandHistoryRecord `json:"cmd_history"`
	Roles               map[string]string      `json:"roles"` // e.g., "punisher": "roleID"
}

func New(filePath string) (*Storage, error) {
	ds, err := datastore.New(filePath)
	if err != nil {
		return nil, err
	}
	return &Storage{ds: ds}, nil
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
