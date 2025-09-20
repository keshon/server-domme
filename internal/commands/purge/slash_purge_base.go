package purge

import (
	"errors"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	ActiveDeletions   = make(map[string]chan struct{})
	ActiveDeletionsMu sync.Mutex

	timePattern = regexp.MustCompile(`(?i)(\d+)([smhdw])`)
)

func stopDeletion(channelID string) {
	ActiveDeletionsMu.Lock()
	defer ActiveDeletionsMu.Unlock()
	if ch, ok := ActiveDeletions[channelID]; ok {
		close(ch)
		delete(ActiveDeletions, channelID)
	}
}

func parseDuration(input string) (time.Duration, error) {
	matches := timePattern.FindAllStringSubmatch(input, -1)
	if matches == nil {
		return 0, errors.New("invalid duration format")
	}

	var total time.Duration
	for _, match := range matches {
		value, _ := strconv.Atoi(match[1])
		unit := match[2]

		switch unit {
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
			return 0, errors.New("unknown time unit: " + unit)
		}
	}

	return total, nil
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
