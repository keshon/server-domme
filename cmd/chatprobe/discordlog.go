package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// message is one line of a conversation copied out of Discord.
type message struct {
	Author string
	At     time.Time
	Text   string
	// Bot marks her own messages.
	Bot bool
}

// Discord's copied header is "Name — 12.09.2026 14:06". The time takes three
// shapes depending on how old the message was when it was copied: a full
// date, "Yesterday at 1:05", or a bare "15:52" for today.
var (
	header     = regexp.MustCompile(`^(.*?)\s*—\s*(\d{2}\.\d{2}\.\d{4} \d{1,2}:\d{2}|(?:Yesterday|Today) at \d{1,2}:\d{2}|\d{1,2}:\d{2})\s*$`)
	appBadge   = "APP"
	fullLayout = "02.01.2006 15:04"
)

// parseLog reads a conversation copied out of Discord. botName is how her
// messages are headed: Discord puts her name on a line of its own, then the
// APP badge, then " — time".
//
// today anchors "Yesterday at" and bare times, since a copied log does not
// say what day it was copied on.
func parseLog(r io.Reader, botName string, today time.Time, loc *time.Location) ([]message, error) {
	var out []message
	var author string
	var at time.Time
	var bot, pendingBot bool

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)

		if trimmed == botName {
			pendingBot = true
			continue
		}
		if pendingBot && trimmed == appBadge {
			continue
		}
		if m := header.FindStringSubmatch(line); m != nil {
			t, err := parseStamp(m[2], today, loc)
			if err != nil {
				return nil, err
			}
			name := strings.TrimSpace(m[1])
			bot = pendingBot && name == ""
			if bot {
				name = botName
			}
			if name == "" {
				return nil, fmt.Errorf("chatprobe: a header with no author: %q", line)
			}
			author, at, pendingBot = name, t, false
			continue
		}
		if pendingBot {
			// Her name was a line of text after all, not a header.
			out = appendText(out, author, at, bot, botName)
			pendingBot = false
		}
		if trimmed == "" || author == "" {
			continue
		}
		out = appendText(out, author, at, bot, trimmed)
	}
	return out, scanner.Err()
}

func appendText(out []message, author string, at time.Time, bot bool, text string) []message {
	if author == "" {
		return out
	}
	return append(out, message{Author: author, At: at, Text: text, Bot: bot})
}

func parseStamp(s string, today time.Time, loc *time.Location) (time.Time, error) {
	if t, err := time.ParseInLocation(fullLayout, s, loc); err == nil {
		return t, nil
	}
	day := today
	if strings.HasPrefix(s, "Yesterday at ") {
		day = today.AddDate(0, 0, -1)
		s = strings.TrimPrefix(s, "Yesterday at ")
	}
	s = strings.TrimPrefix(s, "Today at ")
	clock, err := time.ParseInLocation("15:04", s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("chatprobe: unreadable time %q: %w", s, err)
	}
	y, mo, d := day.Date()
	return time.Date(y, mo, d, clock.Hour(), clock.Minute(), 0, 0, loc), nil
}
