package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout bounds one backend call. Free relays queue requests behind
// whatever else is using the donated server, so this is generous compared to a
// paid endpoint.
const DefaultTimeout = 45 * time.Second

// maxErrorBody caps how much of a failed response is quoted into an error.
// Relay errors are usually one JSON line, but an HTML error page from a proxy
// in front of one is not, and the whole thing would otherwise reach the log.
const maxErrorBody = 300

// Client is one OpenAI-compatible backend: a base URL, a model, and an
// optional key. One type covers pollinations, g4f.space, a local Ollama and a
// paid endpoint, because all four speak the same REST shape — there is no
// per-vendor driver here and adding one would be the wrong fix for a backend
// that differs.
type Client struct {
	// Name identifies the backend in logs and in Pool's scoring. It is not
	// sent anywhere.
	Name string
	// BaseURL is the API root without a trailing slash, e.g.
	// "https://g4f.space/v1". "/chat/completions" is appended to it.
	BaseURL string
	// Model is the model id to request. On g4f.space this is a
	// server-qualified id ("srv_<id>:<model>"); a bare model name there is
	// answered by whichever donated server the router picks, which may not
	// serve it. See catalog.go.
	Model string
	// APIKey is sent as a bearer token when set. The free relays ignore it.
	APIKey string

	HTTP *http.Client
}

// NewClient returns a Client with a timeout-bounded HTTP client.
func NewClient(name, baseURL, model, apiKey string) *Client {
	return &Client{
		Name:    name,
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: DefaultTimeout},
	}
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
		// Delta carries the content when a backend streams despite
		// stream:false. Reading both is what lets one decoder handle a relay
		// that ignores the flag.
		Delta Message `json:"delta"`
	} `json:"choices"`
	// Message is the Ollama-native shape, which some relayed servers answer
	// with instead of the OpenAI one.
	Message Message `json:"message"`
	Done    bool    `json:"done"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Generate implements Provider.
func (c *Client) Generate(ctx context.Context, messages []Message) (string, error) {
	body, err := json.Marshal(chatRequest{Model: c.Model, Messages: messages, Stream: false})
	if err != nil {
		return "", fmt.Errorf("ai: encode request for %s: %w", c.Name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: build request for %s: %w", c.Name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: call %s: %w", c.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai: read %s response: %w", c.Name, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai: %s returned %d: %s", c.Name, resp.StatusCode, snippet(raw))
	}

	reply, err := parseReply(c.Name, raw)
	if err != nil {
		return "", err
	}
	if reply == "" {
		return "", fmt.Errorf("ai: %s: %w", c.Name, ErrEmptyReply)
	}
	return reply, nil
}

// parseReply pulls the assistant text out of a response body. It names the
// backend in its errors itself, so callers return them unwrapped.
//
// Three shapes reach this, all observed from the free relays rather than
// assumed: a single OpenAI chat completion; a stream of line-delimited JSON
// objects, sent even though the request set stream:false; and an
// application-level error carried inside a 200. Anything that does not parse
// as JSON at all is treated as plain text, because some relayed servers answer
// that way when they are proxying a non-OpenAI backend.
func parseReply(name string, raw []byte) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", nil
	}

	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return Clean(text), nil
	}

	decoder := json.NewDecoder(strings.NewReader(text))
	var assembled strings.Builder
	var decoded bool

	for {
		var chunk chatResponse
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			// A body that started as JSON and then stopped parsing has already
			// given us whatever it managed to send; keep that rather than
			// discarding a usable partial reply.
			if decoded {
				break
			}
			return "", fmt.Errorf("ai: decode %s response: %w", name, err)
		}
		decoded = true

		if chunk.Error != nil && chunk.Error.Message != "" {
			return "", fmt.Errorf("ai: %s refused the request: %s", name, chunk.Error.Message)
		}

		if len(chunk.Choices) > 0 {
			assembled.WriteString(chunk.Choices[0].Message.Content)
			assembled.WriteString(chunk.Choices[0].Delta.Content)
		}
		assembled.WriteString(chunk.Message.Content)

		if chunk.Done {
			break
		}
	}

	return Clean(assembled.String()), nil
}

// snippet trims a response body down to something a log line can carry.
func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > maxErrorBody {
		return s[:maxErrorBody] + "…"
	}
	return s
}
