package purge

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// channel is a fake Discord holding one page of a channel's history: it
// answers a fetch of messages with them, and records what is deleted.
type channel struct {
	mu       sync.Mutex
	messages []*discordgo.Message
	deleted  []string
}

func (c *channel) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	body := "{}"
	switch r.Method {
	case http.MethodGet:
		raw, _ := json.Marshal(c.messages)
		body = string(raw)
	case http.MethodDelete:
		parts := strings.Split(r.URL.Path, "/")
		c.deleted = append(c.deleted, parts[len(parts)-1])
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}, nil
}

// "Older than a day" deletes what is older than a day and nothing newer. The
// scheduler had the window inverted and deleted the newest messages; the
// command had it reversed and deleted nothing.
func TestDeleteOlderThanDeletesOnlyTheOld(t *testing.T) {
	now := time.Now()
	c := &channel{messages: []*discordgo.Message{
		{ID: "new", ChannelID: "c", Timestamp: now.Add(-time.Hour)},
		{ID: "old", ChannelID: "c", Timestamp: now.Add(-48 * time.Hour)},
	}}
	s := &discordgo.Session{Client: &http.Client{Transport: c}, Ratelimiter: discordgo.NewRatelimiter()}
	DeleteOlderThan(s, "c", 24*time.Hour, nil)
	if len(c.deleted) != 1 || c.deleted[0] != "old" {
		t.Errorf("deleted %v, want only the message older than a day", c.deleted)
	}
}
