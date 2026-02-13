package mind

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MemoryTypeConversationEvent = "conversation_event"
	MaxRelevantMemories         = 3
)

// AddMemoryEntry appends an episodic memory for the guild (e.g. after summarization or high-emotion event).
func AddMemoryEntry(store *Store, guildID string, entry MemoryEntry) {
	if store == nil || guildID == "" {
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339)
	}
	if entry.Type == "" {
		entry.Type = MemoryTypeConversationEvent
	}
	dir := store.GuildMemoriesDir(guildID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	// One file per entry to avoid rewriting large arrays
	name := time.Now().Format("20060102_150405.000") + ".json"
	path := filepath.Join(dir, name)
	b, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0644)
}

// GetRelevantMemories returns up to limit memories sorted by importance and recency (for system prompt).
func GetRelevantMemories(store *Store, guildID string, limit int) []MemoryEntry {
	if store == nil || guildID == "" || limit <= 0 {
		return nil
	}
	dir := store.GuildMemoriesDir(guildID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var list []MemoryEntry
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var m MemoryEntry
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		list = append(list, m)
	}
	if len(list) == 0 {
		return nil
	}
	// Sort: higher (importance + emotional_weight) and more recent first
	sort.Slice(list, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, list[i].Timestamp)
		tj, _ := time.Parse(time.RFC3339, list[j].Timestamp)
		scoreI := (list[i].Importance + list[i].EmotionalWeight) * recencyFactor(ti, now)
		scoreJ := (list[j].Importance + list[j].EmotionalWeight) * recencyFactor(tj, now)
		return scoreI > scoreJ
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list
}

func recencyFactor(t, now time.Time) float64 {
	if t.IsZero() {
		return 0.5
	}
	days := now.Sub(t).Hours() / 24
	if days < 0 {
		days = 0
	}
	// Decay over ~30 days
	return 1.0 / (1.0 + days/30.0)
}

// FormatMemoriesForPrompt returns a short block for the system prompt (1–3 relevant memories).
func FormatMemoriesForPrompt(store *Store, guildID string, maxChars int) string {
	memories := GetRelevantMemories(store, guildID, MaxRelevantMemories)
	if len(memories) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- Relevant memories ---\n")
	for _, m := range memories {
		b.WriteString("- ")
		b.WriteString(m.Summary)
		b.WriteString("\n")
	}
	s := b.String()
	if maxChars > 0 && len(s) > maxChars {
		return s[:maxChars] + "\n"
	}
	return s
}
