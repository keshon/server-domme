package ai

import (
	"regexp"
	"strings"
)

func isGarbageResponse(s string) bool {
	l := strings.ToLower(s)

	if strings.Contains(l, "<html") {
		return true
	}
	if strings.Contains(l, "not allowed") {
		return true
	}
	if len(strings.TrimSpace(s)) < 5 {
		return true
	}
	return false
}

func truncate(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "..."
	}
	return string(b)
}

func cleanReply(reply string) string {
	reply = strings.TrimSpace(reply)
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	reply = re.ReplaceAllString(reply, "")
	reply = strings.TrimSpace(reply)

	if len(reply) >= 2 {
		quotes := []struct{ open, close string }{
			{`"`, `"`}, {`'`, `'`}, {"“", "”"}, {"‘", "’"},
		}
		for _, q := range quotes {
			if strings.HasPrefix(reply, q.open) && strings.HasSuffix(reply, q.close) {
				reply = strings.TrimSuffix(strings.TrimPrefix(reply, q.open), q.close)
				reply = strings.TrimSpace(reply)
				break
			}
		}
	}

	if len(reply) > 2800 {
		reply = reply[:2800] + "\n\n[truncated]"
	}

	return reply
}
