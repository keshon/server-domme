package mind

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConversationsKeepsRecentTurnsInOrder(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	c.Record("chan", Turn{Username: "ann", Content: "first", At: now})
	c.Record("chan", Turn{Content: "second", At: now.Add(time.Second), FromBot: true})

	got := c.Recent("chan")
	if len(got) != 2 {
		t.Fatalf("got %d turns, want 2", len(got))
	}
	if got[0].Content != "first" || got[1].Content != "second" {
		t.Errorf("turns out of order: %+v", got)
	}
}

// A gap this long means the conversation ended. Replying into yesterday's
// thread as though it were live is the most visible way a chat bot gets it
// wrong.
func TestConversationsDropsStaleTurns(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	c.Record("chan", Turn{Content: "ancient history", At: now.Add(-2 * TurnStaleAfter)})
	c.Record("chan", Turn{Content: "still talking", At: now})

	got := c.Recent("chan")
	if len(got) != 1 {
		t.Fatalf("got %d turns, want only the live one: %+v", len(got), got)
	}
	if got[0].Content != "still talking" {
		t.Errorf("kept the wrong turn: %q", got[0].Content)
	}
}

func TestConversationsBoundsHistoryPerChannel(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	for i := 0; i < maxTurnsPerChannel*3; i++ {
		c.Record("chan", Turn{Content: fmt.Sprint(i), At: now.Add(time.Duration(i) * time.Second)})
	}

	if got := len(c.Recent("chan")); got > maxTurnsPerChannel {
		t.Errorf("kept %d turns, want at most %d", got, maxTurnsPerChannel)
	}
}

func TestConversationsKeepsChannelsApart(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	c.Record("a", Turn{Content: "in a", At: now})
	c.Record("b", Turn{Content: "in b", At: now})

	if got := c.Recent("a"); len(got) != 1 || got[0].Content != "in a" {
		t.Errorf("channel a = %+v", got)
	}
	if got := c.Recent("b"); len(got) != 1 || got[0].Content != "in b" {
		t.Errorf("channel b = %+v", got)
	}
}

func TestConversationsEvictsColdestChannelWhenFull(t *testing.T) {
	c := NewConversations()
	base := time.Now()

	for i := 0; i < maxChannels; i++ {
		c.Record(fmt.Sprintf("chan-%d", i), Turn{
			Content: "hi",
			At:      base.Add(time.Duration(i) * time.Second),
		})
	}
	// chan-0 is the coldest, so the next new channel should displace it.
	c.Record("newcomer", Turn{Content: "hi", At: base.Add(time.Hour)})

	if got := c.Recent("chan-0"); len(got) != 0 {
		t.Errorf("coldest channel survived eviction: %+v", got)
	}
	if got := c.Recent("newcomer"); len(got) != 1 {
		t.Errorf("new channel was not recorded: %+v", got)
	}
}

func TestConversationsForgetClearsAChannel(t *testing.T) {
	c := NewConversations()
	c.Record("chan", Turn{Content: "hi", At: time.Now()})
	c.Forget("chan")
	if got := c.Recent("chan"); len(got) != 0 {
		t.Errorf("Forget left %d turns", len(got))
	}
}

func TestConversationsAreSafeUnderConcurrentUse(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch := fmt.Sprintf("chan-%d", i%4)
			c.Record(ch, Turn{Content: "x", At: now})
			_ = c.Recent(ch)
		}(i)
	}
	wg.Wait()
}
