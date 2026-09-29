package llm

import (
	"strings"
	"unicode/utf8"
)

// Sanitize makes model output postable: trims space, drops control-word
// lines (SKIP, #channel) and @everyone/@here pings, caps length.
//
// It never rewrites meaning: over-long text is cut with an ellipsis marker,
// not summarized.
func Sanitize(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 2000
	}
	lines := strings.Split(s, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		upper := strings.ToUpper(t)
		if upper == "SKIP" || strings.HasPrefix(upper, "SKIP ") {
			continue
		}
		if strings.HasPrefix(t, "#") && !strings.Contains(t, " ") {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	out = strings.ReplaceAll(out, "@everyone", "@\u200beveryone")
	out = strings.ReplaceAll(out, "@here", "@\u200bhere")
	if utf8.RuneCountInString(out) > maxLen {
		runes := []rune(out)
		out = string(runes[:maxLen-1]) + "…"
	}
	return out
}

// StripMention removes bot mentions (<@id>, <@!id>) and trims the remainder.
func StripMention(s, botID string) string {
	out := s
	if botID != "" {
		out = strings.ReplaceAll(out, "<@"+botID+">", " ")
		out = strings.ReplaceAll(out, "<@!"+botID+">", " ")
	}
	return strings.TrimSpace(strings.Join(strings.Fields(out), " "))
}
