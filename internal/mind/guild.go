package mind

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GuildState holds guild-specific mind data and short-term message buffer.
type GuildState struct {
	GuildID       string
	root          string
	mu            sync.RWMutex
	emotions      *Emotions
	activity      *Activity
	mediumMem     []byte
	people        map[string]*Person
	shortBuffer   []ShortMessage
	shortCharCount int
	maxShort      int
}

// NewGuildState creates state for a guild. root = data/mind, guildID = Discord guild ID.
func NewGuildState(root, guildID string) *GuildState {
	if root == "" {
		root = "data/mind"
	}
	return &GuildState{
		GuildID:  guildID,
		root:     filepath.Join(root, "guilds", guildID),
		people:   make(map[string]*Person),
		maxShort: 80,
	}
}

// Load reads guild files from disk.
func (g *GuildState) Load() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// emotions
	if b, err := os.ReadFile(filepath.Join(g.root, GuildEmotions)); err == nil {
		var e Emotions
		if json.Unmarshal(b, &e) == nil {
			g.emotions = &e
		}
	}
	if g.emotions == nil {
		g.emotions = &Emotions{}
	}

	// activity
	if b, err := os.ReadFile(filepath.Join(g.root, GuildActivity)); err == nil {
		var a Activity
		if json.Unmarshal(b, &a) == nil {
			g.activity = &a
		}
	}
	if g.activity == nil {
		g.activity = &Activity{}
	}
	if g.activity.LastActivityUpdate.IsZero() && !g.activity.LastMsgAt.IsZero() {
		g.activity.LastActivityUpdate = g.activity.LastMsgAt
	}

	// medium_memory.md
	if b, err := os.ReadFile(filepath.Join(g.root, GuildMediumMemory)); err == nil {
		g.mediumMem = b
	}

	// people/
	peopleDir := filepath.Join(g.root, "people")
	if ents, err := os.ReadDir(peopleDir); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if len(name) > 5 && name[len(name)-5:] == ".json" {
				userID := name[:len(name)-5]
				if b, err := os.ReadFile(filepath.Join(peopleDir, name)); err == nil {
					var p Person
					if json.Unmarshal(b, &p) == nil {
						p.UserID = userID
						g.people[userID] = &p
					}
				}
			}
		}
	}

	return nil
}

// GetShortCharCount returns current character count of short buffer (for summarization trigger).
func (g *GuildState) GetShortCharCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.shortCharCount
}

// NeedSummarization returns true if short memory exceeds threshold.
func (g *GuildState) NeedSummarization() bool {
	return g.GetShortCharCount() > ShortMemorySummarizeThreshold
}

// ClearShortBuffer clears buffer and resets shortCharCount (call after summarization).
func (g *GuildState) ClearShortBuffer() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.shortBuffer = nil
	g.shortCharCount = 0
}

// PushMessage adds a message to short-term buffer and bumps activity. Updates shortCharCount.
func (g *GuildState) PushMessage(m ShortMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := m.At

	g.shortBuffer = append(g.shortBuffer, m)
	g.shortCharCount += len(m.Content) + len(m.Username) + 32
	if len(g.shortBuffer) > g.maxShort {
		g.shortBuffer = g.shortBuffer[len(g.shortBuffer)-g.maxShort:]
		g.recomputeShortCharCount()
	}

	if g.activity == nil {
		g.activity = &Activity{}
	}
	if m.Role == "user" {
		g.activity.ConsecutiveBotReplies = 0
		g.activity.AwaitingReply = false
	} else if m.Role == "assistant" {
		if len(g.shortBuffer) >= 2 && g.shortBuffer[len(g.shortBuffer)-2].Role == "assistant" {
			g.activity.ConsecutiveBotReplies++
		} else {
			g.activity.ConsecutiveBotReplies = 1
		}
	}
	g.activity.Score += 1.0
	if g.activity.Score > 100 {
		g.activity.Score = 100
	}
	g.activity.LastMsgAt = now
	g.activity.LastActivityUpdate = now
	g.activity.LastChannelID = m.ChannelID
}

// SetLastLLMCallAt sets per-guild LLM cooldown timestamp.
func (g *GuildState) SetLastLLMCallAt(t time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activity == nil {
		g.activity = &Activity{}
	}
	g.activity.LastLLMCallAt = t
}

func (g *GuildState) recomputeShortCharCount() {
	g.shortCharCount = 0
	for _, m := range g.shortBuffer {
		g.shortCharCount += len(m.Content) + len(m.Username) + 32
	}
}

// GetShortBuffer returns a copy of recent messages (oldest first).
func (g *GuildState) GetShortBuffer() []ShortMessage {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]ShortMessage, len(g.shortBuffer))
	copy(out, g.shortBuffer)
	return out
}

// GetActivity returns a copy of activity state.
func (g *GuildState) GetActivity() Activity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.activity == nil {
		return Activity{}
	}
	return *g.activity
}

// SetLastTickAt updates last tick time (scheduler).
func (g *GuildState) SetLastTickAt(t time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activity == nil {
		g.activity = &Activity{}
	}
	g.activity.LastTickAt = t
}

// GetEmotions returns current emotions (copy).
func (g *GuildState) GetEmotions() *Emotions {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.emotions == nil {
		return &Emotions{}
	}
	e := *g.emotions
	return &e
}

// SetEmotions overwrites emotions and persists.
func (g *GuildState) SetEmotions(e *Emotions) {
	if e == nil {
		return
	}
	g.mu.Lock()
	g.emotions = e
	g.mu.Unlock()
	g.saveEmotions(e)
}

func (g *GuildState) saveEmotions(e *Emotions) {
	path := filepath.Join(g.root, GuildEmotions)
	os.MkdirAll(filepath.Dir(path), 0755)
	b, _ := json.MarshalIndent(e, "", "  ")
	_ = os.WriteFile(path, b, 0644)
}

// GetMediumMemory returns medium_memory.md content (copy).
func (g *GuildState) GetMediumMemory() []byte {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if len(g.mediumMem) == 0 {
		return nil
	}
	out := make([]byte, len(g.mediumMem))
	copy(out, g.mediumMem)
	return out
}

// SetMediumMemory overwrites medium memory (e.g. after summarization). Never call from arbitrary LLM output without validation.
func (g *GuildState) SetMediumMemory(b []byte) error {
	g.mu.Lock()
	g.mediumMem = b
	g.mu.Unlock()
	path := filepath.Join(g.root, GuildMediumMemory)
	os.MkdirAll(filepath.Dir(path), 0755)
	return os.WriteFile(path, b, 0644)
}

// GetPerson returns person model for userID (copy or nil).
func (g *GuildState) GetPerson(userID string) *Person {
	g.mu.RLock()
	defer g.mu.RUnlock()
	p := g.people[userID]
	if p == nil {
		return nil
	}
	pc := *p
	return &pc
}

// SetPerson saves person model.
func (g *GuildState) SetPerson(p *Person) {
	if p == nil {
		return
	}
	g.mu.Lock()
	g.people[p.UserID] = p
	g.mu.Unlock()
	path := filepath.Join(g.root, "people", p.UserID+".json")
	os.MkdirAll(filepath.Dir(path), 0755)
	b, _ := json.MarshalIndent(p, "", "  ")
	_ = os.WriteFile(path, b, 0644)
}

// ApplyActivityDecay applies exponential decay by real time: score *= exp(-k * elapsed).
// Call from Tick and ensure LastActivityUpdate is set on bump (PushMessage).
func (g *GuildState) ApplyActivityDecay(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activity == nil {
		return
	}
	elapsed := now.Sub(g.activity.LastActivityUpdate).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	g.activity.Score *= math.Exp(-ActivityDecayK * elapsed)
	if g.activity.Score < 0 {
		g.activity.Score = 0
	}
	g.activity.LastActivityUpdate = now
	g.activity.LastTickAt = now
}

// SaveActivity persists activity state.
func (g *GuildState) SaveActivity() {
	g.mu.RLock()
	a := g.activity
	g.mu.RUnlock()
	if a == nil {
		return
	}
	path := filepath.Join(g.root, GuildActivity)
	os.MkdirAll(filepath.Dir(path), 0755)
	b, _ := json.MarshalIndent(a, "", "  ")
	_ = os.WriteFile(path, b, 0644)
}

// SetLastSpokeAt records when the bot last sent a message (for RecentlySpokePenalty).
func (g *GuildState) SetLastSpokeAt(t time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activity == nil {
		g.activity = &Activity{}
	}
	g.activity.LastSpokeAt = t
}

// SetAwaitingReply sets PendingResponse (bot asked a question; block proactive until user reply or timeout).
func (g *GuildState) SetAwaitingReply(since time.Time, topic string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activity == nil {
		g.activity = &Activity{}
	}
	g.activity.AwaitingReply = true
	g.activity.AwaitingReplySince = since
	g.activity.AwaitingTopic = topic
}

// ClearAwaitingReply clears PendingResponse (user replied or timeout).
func (g *GuildState) ClearAwaitingReply() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activity == nil {
		return
	}
	g.activity.AwaitingReply = false
}

// SetLastAIIntent records last bot action/topic to avoid proactive repeat.
func (g *GuildState) SetLastAIIntent(action, topic string, at time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activity == nil {
		g.activity = &Activity{}
	}
	g.activity.LastAIAction = action
	g.activity.LastAITopic = topic
	g.activity.LastAITimestamp = at
}
