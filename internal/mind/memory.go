package mind

import (
	"fmt"
	"log"
	"strings"

	"server-domme/internal/ai"
)

// SummarizeMemoryPrompt instructs the LLM to merge short-term into medium memory. No personality—just summarization.
const SummarizeMemoryPrompt = `You are a summarizer. Merge the following "Current medium memory" with the "Recent conversation" into a single, concise guild context summary in the same tone. Output ONLY the new summary text, no preamble. Keep important names, events, and social dynamics. Maximum 2 short paragraphs.`

// SummarizeMemory calls LLM to merge shortBuffer + current medium_memory into updated medium_memory, then clears short buffer.
// If store is non-nil, adds an episodic memory entry for this summarization round.
func SummarizeMemory(provider ai.Provider, g *GuildState, guildID string, store *Store) error {
	medium := g.GetMediumMemory()
	shortBuf := g.GetShortBuffer()

	var participants []string
	seen := make(map[string]bool)
	for _, m := range shortBuf {
		if m.Role == "user" && m.UserID != "" && !seen[m.UserID] {
			seen[m.UserID] = true
			participants = append(participants, m.Username)
		}
	}

	var recent strings.Builder
	for _, m := range shortBuf {
		if m.Role == "user" {
			recent.WriteString(m.Username)
			recent.WriteString(": ")
		} else {
			recent.WriteString("Assistant: ")
		}
		recent.WriteString(m.Content)
		recent.WriteString("\n")
	}

	content := "Current medium memory:\n" + string(medium) + "\n\nRecent conversation:\n" + recent.String()
	if len(content) > 8000 {
		content = content[len(content)-8000:]
	}

	log.Printf("[MIND] summarization prompt guild=%s system_len=%d user_len=%d", guildID, len(SummarizeMemoryPrompt), len(content))
	preview := content
	if len(preview) > 300 {
		preview = preview[:300] + "..."
	}
	log.Printf("[MIND] summarization user_content: %s", preview)

	messages := []ai.Message{
		{Role: "system", Content: SummarizeMemoryPrompt},
		{Role: "user", Content: content},
	}
	out, err := provider.Generate(messages)
	if err != nil {
		return err
	}

	merged := strings.TrimSpace(out)
	if merged == "" {
		return fmt.Errorf("summarizer returned empty")
	}
	log.Printf("[MIND] summarization result_len=%d preview: %s", len(merged), truncateForLog(merged, 200))
	if err := g.SetMediumMemory([]byte(merged)); err != nil {
		return err
	}
	g.ClearShortBuffer()

	if store != nil {
		summary := merged
		if len(summary) > 400 {
			summary = summary[:400] + "..."
		}
		emo := g.GetEmotions()
		weight := (emo.Engagement + emo.Joy - emo.Anger*0.5) / 2
		if weight < 0 {
			weight = 0
		}
		if weight > 1 {
			weight = 1
		}
		AddMemoryEntry(store, guildID, MemoryEntry{
			Type:            MemoryTypeConversationEvent,
			Summary:         summary,
			Participants:    participants,
			EmotionalWeight: weight,
			Importance:      0.6,
		})
	}

	return nil
}
