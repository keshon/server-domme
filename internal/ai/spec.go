package ai

import (
	"fmt"
	"strings"
)

// backendSpecFields is the shape of one entry in Options.Extra.
const backendSpecFields = "name|baseURL|model[|apiKey]"

// specSeparator divides the fields of a backend spec.
//
// A pipe rather than a comma because the list of specs is itself
// comma-separated by the environment, and because it does not appear in URLs,
// model ids or the base64-ish API keys these endpoints issue.
const specSeparator = "|"

// ParseBackendSpec turns "name|baseURL|model" or "name|baseURL|model|key"
// into a Client.
//
// The name is the operator's label and is what appears in logs and in /chat
// status, so it should say which endpoint this is rather than which model —
// the model is already reported alongside it.
func ParseBackendSpec(spec string) (*Client, error) {
	parts := strings.Split(spec, specSeparator)
	if len(parts) < 3 || len(parts) > 4 {
		return nil, fmt.Errorf("ai: backend spec %q has %d fields, want %s",
			spec, len(parts), backendSpecFields)
	}

	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	name, baseURL, model := parts[0], parts[1], parts[2]
	if name == "" || baseURL == "" || model == "" {
		return nil, fmt.Errorf("ai: backend spec %q needs %s", spec, backendSpecFields)
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("ai: backend spec %q: base URL needs a scheme", spec)
	}

	var apiKey string
	if len(parts) == 4 {
		apiKey = parts[3]
	}

	return NewClient(name, baseURL, model, apiKey), nil
}
