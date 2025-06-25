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
	PrefPrefix          string                 `json:"pref_prefix"`
	UseCache            bool                   `json:"use_cache"`
	CommandMode         string                 `json:"cmd_mode"`
	CommandsHistoryList []CommandHistoryRecord `json:"cmd_history"`
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
			PrefPrefix:          "",
			UseCache:            false,
			CommandsHistoryList: []CommandHistoryRecord{},
		}
		s.ds.Add(guildID, newRecord)
		return newRecord, nil
	}

	// Try to convert `data` (map[string]interface{}) into JSON format
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error marshalling data: %w", err)
	}

	// Unmarshal JSON data into the Record struct
	var record Record
	err = json.Unmarshal(jsonData, &record)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling to *Record: %w", err)
	}

	if len(record.CommandsHistoryList) > commandHistoryLimit {
		record.CommandsHistoryList = record.CommandsHistoryList[len(record.CommandsHistoryList)-20:]
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
