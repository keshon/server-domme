package commands

import (
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	ActiveDeletions   = make(map[string]chan struct{})
	ActiveDeletionsMu sync.Mutex
)

func stopNuke(channelID string) {
	ActiveDeletionsMu.Lock()
	defer ActiveDeletionsMu.Unlock()
	if ch, ok := ActiveDeletions[channelID]; ok {
		close(ch)
		delete(ActiveDeletions, channelID)
	}
}

func DeleteMessages(s *discordgo.Session, channelID string, startTime, endTime *time.Time, stopChan <-chan struct{}) {
	var lastID string

	for {
		select {
		case <-stopChan:
			return
		default:
		}

		msgs, err := s.ChannelMessages(channelID, 100, lastID, "", "")
		if err != nil || len(msgs) == 0 {
			break
		}

		for _, msg := range msgs {
			select {
			case <-stopChan:
				return
			default:
			}

			if startTime != nil && msg.Timestamp.Before(*startTime) {
				continue
			}
			if endTime != nil && msg.Timestamp.After(*endTime) {
				continue
			}

			_ = s.ChannelMessageDelete(channelID, msg.ID)
			time.Sleep(300 * time.Millisecond)
		}

		lastID = msgs[len(msgs)-1].ID
		if len(msgs) < 100 {
			break
		}
	}
}
