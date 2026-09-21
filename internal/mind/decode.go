package mind

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// decodeObject finds the JSON object in a model's reply and decodes it.
//
// Models wrap what they were asked for: a ```json fence, a sentence before it,
// a remark after. The object is taken from the first "{" to the last "}", and
// anything outside is ignored. It is decoded into a map rather than a struct
// because the values come back in whatever type the model felt like — a
// number as "24", a boolean as "yes" — and a struct would reject the whole
// object over one of them.
func decodeObject(reply string) (map[string]any, bool) {
	start := strings.Index(reply, "{")
	end := strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(reply[start:end+1]), &out); err != nil {
		return looseObject(reply[start : end+1])
	}
	return out, true
}

// looseLine is one "key": value line of an object written a key per line.
var looseLine = regexp.MustCompile(`^\s*"([A-Za-z_]+)"\s*:\s*(.*?)\s*,?\s*$`)

// looseObject reads an object that is not valid JSON but is laid out the way
// models lay one out: a key per line, string values quoted.
//
// The usual fault is a quote inside a value left unescaped — "read": "he is
// testing her with a "haha"-shielded compliment" — which was seen in
// production and cost a whole appraisal. Read a line at a time, the value is
// everything between the first and last quote on its line, so the inner
// ones survive. Lists and nested objects are not attempted; the appraisal
// has none, and reflection is asked again rather than half read.
func looseObject(text string) (map[string]any, bool) {
	out := make(map[string]any)
	for _, line := range strings.Split(text, "\n") {
		m := looseLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value := m[2]
		if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) && len(value) >= 2 {
			out[m[1]] = strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
			continue
		}
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			out[m[1]] = f
		} else {
			out[m[1]] = value
		}
	}
	return out, len(out) > 0
}

// str reads a string field, trimmed. A missing field, null, or one of the
// words a model writes for "nothing" all come back empty.
func str(obj map[string]any, key string) string {
	var s string
	switch v := obj[key].(type) {
	case string:
		s = v
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		s = strconv.FormatBool(v)
	default:
		return ""
	}
	s = strings.TrimSpace(s)
	switch strings.ToLower(strings.Trim(s, ".")) {
	case "", "none", "null", "n/a", "nothing", "-", "no":
		return ""
	}
	return s
}

// num reads a number field, accepting one written as a string.
func num(obj map[string]any, key string) float64 {
	switch v := obj[key].(type) {
	case float64:
		return v
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return f
		}
	}
	return 0
}

// objects reads a field holding a list of objects.
func objects(obj map[string]any, key string) []map[string]any {
	list, _ := obj[key].([]any)
	var out []map[string]any
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// numbers reads a field holding a list of numbers, accepting strings.
func numbers(obj map[string]any, key string) []int {
	list, _ := obj[key].([]any)
	var out []int
	for _, item := range list {
		switch v := item.(type) {
		case float64:
			out = append(out, int(v))
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// texts reads a field holding a list of strings, reporting whether the field
// was there at all: an empty list is a decision, a missing one is not.
func texts(obj map[string]any, key string) ([]string, bool) {
	list, ok := obj[key].([]any)
	if !ok {
		return nil, false
	}
	var out []string
	for _, item := range list {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out, true
}
