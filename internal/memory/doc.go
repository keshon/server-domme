package memory

import (
	"strings"
	"time"
)

// frontMatter is the "---" block at the top of a file: one "key: value" per
// line. Deliberately not YAML — nothing here needs more than flat strings, and
// a hand-edited file with a stray colon should still read.
const frontMatter = "---"

// field is one front matter entry, kept in order so a rewritten file comes out
// in the order a person reading it expects.
type field struct{ key, value string }

// parseDoc splits a file into its front matter and its body.
func parseDoc(text string) (map[string]string, string) {
	fields := make(map[string]string)
	if !strings.HasPrefix(text, frontMatter+"\n") {
		return fields, strings.TrimSpace(text)
	}
	// Searched from a newline put in front, so an empty block — "---" then
	// "---" straight after, as a hand-cleaned file can be left — closes
	// where it opens. Without it the whole file read as body, the two
	// lines were kept as text, and every rewrite carried them along.
	rest := "\n" + text[len(frontMatter)+1:]
	end := strings.Index(rest, "\n"+frontMatter)
	if end < 0 {
		return fields, strings.TrimSpace(text)
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	body := rest[end+1+len(frontMatter):]
	return fields, strings.TrimSpace(body)
}

// renderDoc writes front matter and a body back out. Empty values are left
// out rather than written as "key:".
func renderDoc(fields []field, body string) string {
	var b strings.Builder
	b.WriteString(frontMatter + "\n")
	for _, f := range fields {
		if v := oneLine(f.value); v != "" {
			b.WriteString(f.key + ": " + v + "\n")
		}
	}
	b.WriteString(frontMatter + "\n\n")
	if body = strings.TrimSpace(body); body != "" {
		b.WriteString(body + "\n")
	}
	return b.String()
}

// sections splits a body on "## " headings. Text before the first heading is
// under "". Headings are matched case-insensitively, since a person may have
// edited the file by hand.
func sections(body string) map[string]string {
	out := make(map[string]string)
	current := ""
	var b strings.Builder
	flush := func() {
		if text := strings.TrimSpace(b.String()); text != "" {
			out[current] = text
		}
		b.Reset()
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.ToLower(strings.TrimSpace(line[3:]))
			continue
		}
		b.WriteString(line + "\n")
	}
	flush()
	return out
}

// bullets returns the "- " items of a section, in order.
func bullets(section string) []string {
	var out []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			out = append(out, strings.TrimSpace(line[2:]))
		}
	}
	return out
}

// parseTime reads a time written by formatTime, or the zero time.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}

// formatTime writes a time for front matter, or "" for the zero time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
