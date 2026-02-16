package mind

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"server-domme/internal/ai"
)

// TokenBudget — approximate token limits per section. LLMs ~4 chars/token for English.
const (
	BudgetCoreIdentity     = 600 // tokens
	BudgetBiology          = 250
	BudgetWorldview        = 200
	BudgetMediumMemory     = 400
	BudgetRelevantMemories = 200 // episodic memories in system prompt
	BudgetPeopleSummary    = 400 // total for all mentioned people
	BudgetShortContext     = 800 // last N messages; dynamic
	CharsPerToken          = 4
)

// TokenBudgetManager enforces limits. Never send full chat history or raw logs.
type TokenBudgetManager struct {
	MaxCoreIdentity     int
	MaxBiology          int
	MaxWorldview        int
	MaxMediumMemory     int
	MaxRelevantMemories int
	MaxPeopleSummary    int
	MaxShortContext     int
}

// DefaultTokenBudget returns default limits.
func DefaultTokenBudget() TokenBudgetManager {
	return TokenBudgetManager{
		MaxCoreIdentity:     BudgetCoreIdentity * CharsPerToken,
		MaxBiology:          BudgetBiology * CharsPerToken,
		MaxWorldview:        BudgetWorldview * CharsPerToken,
		MaxMediumMemory:     BudgetMediumMemory * CharsPerToken,
		MaxRelevantMemories: BudgetRelevantMemories * CharsPerToken,
		MaxPeopleSummary:    BudgetPeopleSummary * CharsPerToken,
		MaxShortContext:     BudgetShortContext * CharsPerToken,
	}
}

// TrimToChars truncates s to maxChars, trying to cut at word boundary.
func TrimToChars(s string, maxChars int) string {
	if maxChars <= 0 || len(s) <= maxChars {
		return s
	}
	// Safe for UTF-8
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	out := string(r[:maxChars])
	lastSpace := strings.LastIndex(out, " ")
	if lastSpace > maxChars/2 {
		return strings.TrimSpace(out[:lastSpace])
	}
	return strings.TrimSpace(out)
}

// BuildSystemPrompt builds the system prompt. Identity + behavioral directives + guild context + relevant memories + current feeling.
func BuildSystemPrompt(core *Core, g *GuildState, store *Store, budget TokenBudgetManager) string {
	var b strings.Builder

	// 1. Identity — never trim
	ident := core.GetIdentityMD()
	if len(ident) > 0 {
		b.WriteString(string(ident))
		b.WriteString("\n\n")
	}

	// 2. Behavioral directives (interpreted from biology + worldview; no raw numbers)
	bio := core.GetBiology()
	w := core.GetWorldview()
	b.WriteString(BuildBehaviorDirectives(bio, w))
	b.WriteString("\n")

	// 3. Medium memory — trim to budget only
	if mm := g.GetMediumMemory(); len(mm) > 0 {
		b.WriteString("--- Guild context ---\n")
		b.WriteString(TrimToChars(string(mm), budget.MaxMediumMemory))
		b.WriteString("\n")
	}

	// 4. Relevant episodic memories (1–3)
	if store != nil && budget.MaxRelevantMemories > 0 {
		if mem := FormatMemoriesForPrompt(store, g.GuildID, budget.MaxRelevantMemories); mem != "" {
			b.WriteString(mem)
		}
	}

	// 5. Current feeling (plain phrase, no numbers)
	feeling := CurrentFeelingPhrase(g.GetEmotions())
	if feeling != "" {
		b.WriteString("--- Current state ---\n")
		b.WriteString(feeling)
		b.WriteString("\n")
	}

	return b.String()
}

// BuildMessagesForLLM returns ai.Message slice: system (identity + directives + relationship + people) + recent messages.
func BuildMessagesForLLM(core *Core, g *GuildState, shortBuf []ShortMessage, budget TokenBudgetManager, store *Store) []ai.Message {
	sys := BuildSystemPrompt(core, g, store, budget)

	// Current speaker: last user in shortBuf (most recent message from a user)
	var currentSpeakerID string
	for i := len(shortBuf) - 1; i >= 0; i-- {
		if shortBuf[i].Role == "user" && shortBuf[i].UserID != "" {
			currentSpeakerID = shortBuf[i].UserID
			break
		}
	}
	if currentSpeakerID != "" {
		p := g.GetPerson(currentSpeakerID)
		if p != nil {
			sys += "\n" + BuildRelationshipContext(p.Affinity, p.Trust, p.Irritation)
		}
	}

	// People summaries — trim to budget
	seen := make(map[string]bool)
	var peopleParts []string
	peopleChars := 0
	for i := len(shortBuf) - 1; i >= 0; i-- {
		uid := shortBuf[i].UserID
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		p := g.GetPerson(uid)
		if p != nil && p.Summary != "" {
			part := fmt.Sprintf("[%s]: %s", uid, TrimToChars(p.Summary, 200))
			if peopleChars+len(part) > budget.MaxPeopleSummary {
				break
			}
			peopleParts = append(peopleParts, part)
			peopleChars += len(part)
		}
	}
	if len(peopleParts) > 0 {
		sys += "\n--- People ---\n" + strings.Join(peopleParts, "\n") + "\n"
	}

	msgs := []ai.Message{{Role: "system", Content: sys}}

	// Short context: last N messages, trim to budget
	var shortChars int
	start := len(shortBuf) - 1
	for start >= 0 {
		m := shortBuf[start]
		line := m.Username + ": " + m.Content
		if m.Role == "assistant" {
			line = "Assistant: " + m.Content
		}
		if shortChars+len(line) > budget.MaxShortContext {
			break
		}
		shortChars += len(line)
		start--
	}
	for i := start + 1; i < len(shortBuf); i++ {
		m := shortBuf[i]
		role := "user"
		content := m.Username + ": " + m.Content
		if m.Role == "assistant" {
			role = "assistant"
			content = m.Content
		}
		msgs = append(msgs, ai.Message{Role: role, Content: content})
	}

	return msgs
}

// EstimateTokens rough estimate (UTF-8 runes / 4).
func EstimateTokens(s string) int {
	return utf8.RuneCountInString(s) / CharsPerToken
}
