package mind

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"server-domme/internal/ai"
)

// TokenBudget — approximate token limits per section. LLMs ~4 chars/token for English.
const (
	BudgetCoreIdentity  = 600   // tokens
	BudgetBiology       = 150
	BudgetWorldview     = 100
	BudgetMediumMemory  = 400
	BudgetPeopleSummary = 300   // total for all mentioned people
	BudgetShortContext  = 800   // last N messages; dynamic
	CharsPerToken       = 4
)

// TokenBudgetManager enforces limits. Never send full chat history or raw logs.
type TokenBudgetManager struct {
	MaxCoreIdentity  int
	MaxBiology       int
	MaxWorldview     int
	MaxMediumMemory  int
	MaxPeopleSummary int
	MaxShortContext  int
}

// DefaultTokenBudget returns default limits.
func DefaultTokenBudget() TokenBudgetManager {
	return TokenBudgetManager{
		MaxCoreIdentity:  BudgetCoreIdentity * CharsPerToken,
		MaxBiology:       BudgetBiology * CharsPerToken,
		MaxWorldview:     BudgetWorldview * CharsPerToken,
		MaxMediumMemory:  BudgetMediumMemory * CharsPerToken,
		MaxPeopleSummary: BudgetPeopleSummary * CharsPerToken,
		MaxShortContext:  BudgetShortContext * CharsPerToken,
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

// BuildSystemPrompt builds the system prompt from core + guild. Core (identity, biology, worldview) is not trimmed.
func BuildSystemPrompt(core *Core, g *GuildState, budget TokenBudgetManager) string {
	var b strings.Builder

	// 1. Identity — never trim; it's the core personality
	ident := core.GetIdentityMD()
	if len(ident) > 0 {
		b.WriteString(string(ident))
		b.WriteString("\n\n")
	}

	// 2. Biology (fixed; LLM-readable keys)
	bio := core.GetBiology()
	b.WriteString("--- Biology (fixed) ---\n")
	b.WriteString(fmt.Sprintf("temperament=%s age=%d speech_style=%s dominance=%.2f emotional_reactivity=%.2f\n",
		bio.Temperament, bio.Age, bio.SpeechStyle, bio.Dominance, bio.EmoReact))

	// 3. Worldview
	w := core.GetWorldview()
	b.WriteString("--- Worldview ---\n")
	b.WriteString(fmt.Sprintf("trust_in_people=%.2f cynicism=%.2f openness=%.2f loyalty_bias=%.2f\n",
		w.TrustInPeople, w.Cynicism, w.Openness, w.LoyaltyBias))

	// 4. Medium memory (trim to budget)
	if mm := g.GetMediumMemory(); len(mm) > 0 {
		b.WriteString("--- Guild context ---\n")
		b.WriteString(TrimToChars(string(mm), budget.MaxMediumMemory))
		b.WriteString("\n")
	}

	return b.String()
}

// BuildMessagesForLLM returns ai.Message slice: system (from BuildSystemPrompt) + recent messages.
// People summaries for authors in recent msgs are appended to system if within budget.
func BuildMessagesForLLM(core *Core, g *GuildState, shortBuf []ShortMessage, budget TokenBudgetManager) []ai.Message {
	sys := BuildSystemPrompt(core, g, budget)

	// Append people summaries (for users in shortBuf)
	seen := make(map[string]bool)
	var peopleParts []string
	for i := len(shortBuf) - 1; i >= 0 && len(peopleParts)*50 < budget.MaxPeopleSummary; i-- {
		uid := shortBuf[i].UserID
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		p := g.GetPerson(uid)
		if p != nil && p.Summary != "" {
			peopleParts = append(peopleParts, fmt.Sprintf("[%s]: %s", uid, TrimToChars(p.Summary, 200)))
		}
	}
	if len(peopleParts) > 0 {
		sys += "\n--- People ---\n" + strings.Join(peopleParts, "\n") + "\n"
	}
	sys = TrimToChars(sys, budget.MaxCoreIdentity+budget.MaxBiology+budget.MaxWorldview+budget.MaxMediumMemory+budget.MaxPeopleSummary)

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
