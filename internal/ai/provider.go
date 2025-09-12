package ai

import (
	"fmt"
	"server-domme/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Provider interface {
	Generate(messages []Message) (string, error)
}

func DefaultProvider() Provider {
	engine := config.New().AIProvider
	switch engine {
	case "pollinations":
		return NewPollinationsProvider()
	case "g4f", "":
		// fallback
		return NewG4FProvider()
	default:
		panic(fmt.Sprintf("unsupported AI_PROVIDER: %s", engine))
	}
}
