package message

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"server-domme/internal/ai"
	"server-domme/internal/config"
	"server-domme/internal/core"
)

type ChatCommand struct{}

func (c *ChatCommand) Name() string        { return "mention bot" }
func (c *ChatCommand) Description() string { return "Talk to the bot when it is mentioned" }
func (c *ChatCommand) Aliases() []string   { return []string{} }
func (c *ChatCommand) Group() string       { return "chat" }
func (c *ChatCommand) Category() string    { return "💬 Chat" }
func (c *ChatCommand) RequireAdmin() bool  { return false }
func (c *ChatCommand) RequireDev() bool    { return false }

// Message handler
func (c *ChatCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.MessageContext)
	if !ok {
		return nil
	}

	user := context.Event.Author.Username
	displayName := context.Event.Author.DisplayName() // public name
	userID := context.Event.Author.ID
	channelID := context.Event.ChannelID
	msg := strings.TrimSpace(context.Event.Content)

	log.Printf("[CHAT] %s (%s) @ %s: %s", user, userID, channelID, msg)

	if context.Event.GuildID == "" {
		_, err := context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, I don't chat in DMs. Speak to me on a server channel.", displayName))
		return err
	}

	if msg == "" {
		_, err := context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, speak or be silent forever.", displayName))
		return err
	}

	// Prepend "User <PublicName>: " for AI context
	userContent := fmt.Sprintf("User %s: %s", displayName, msg)
	convos.add(channelID, "user", userContent)

	history := convos.get(channelID)

	// Build system prompt
	cfg := config.New()
	file, err := os.Open(cfg.AIPromtPath)
	if err != nil {
		log.Printf("[ERROR] Failed to open system prompt: %v", err)
		return err
	}
	defer file.Close()

	promptBytes, _ := io.ReadAll(file)
	systemPrompt := string(promptBytes)
	log.Printf("[DEBUG] System prompt loaded (%d chars)", len(systemPrompt))

	// Prepare AI messages
	messages := []ai.Message{{Role: "system", Content: systemPrompt}}
	for _, m := range history {
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		messages = append(messages, ai.Message{Role: role, Content: m.Content})
	}

	// Call AI engine
	client := ai.DefaultProvider()
	reply, err := client.Generate(messages)
	if err != nil {
		log.Printf("[ERROR] AI request failed: %v", err)
		_, sendErr := context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("Something went wrong %s, I broke trying to think 🤯", displayName))
		return sendErr
	}

	convos.add(channelID, "assistant", reply)
	log.Printf("[CHAT] AI reply to %s (%s) @ %s: %s", user, userID, channelID, reply)

	// Send reply (respect 2000 char limit)
	if len(reply) > 2000 {
		for _, chunk := range splitMessage(reply, 2000) {
			_, sendErr := context.Session.ChannelMessageSend(channelID, chunk)
			if sendErr != nil {
				return sendErr
			}
			time.Sleep(200 * time.Millisecond)
		}
		return nil
	}

	_, err = context.Session.ChannelMessageSend(channelID, reply)
	if err != nil {
		log.Printf("[ERROR] Failed to send reply: %v", err)
	}
	return err
}

// Conversation store
type convoMsg struct {
	Role    string
	Content string
}

type convoStore struct {
	mu       sync.Mutex
	store    map[string][]convoMsg
	maxMsgs  int
	maxChars int
}

var convos = &convoStore{
	store:    map[string][]convoMsg{},
	maxMsgs:  40,
	maxChars: 14000,
}

func (c *convoStore) add(channelID, role, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	list := append(c.store[channelID], convoMsg{Role: role, Content: content})
	if len(list) > c.maxMsgs {
		list = list[len(list)-c.maxMsgs:]
	}

	total := 0
	for i := len(list) - 1; i >= 0; i-- {
		total += len(list[i].Content)
		if total > c.maxChars {
			list = list[i+1:]
			break
		}
	}

	c.store[channelID] = list
}

func (c *convoStore) get(channelID string) []convoMsg {
	c.mu.Lock()
	defer c.mu.Unlock()
	dst := make([]convoMsg, len(c.store[channelID]))
	copy(dst, c.store[channelID])
	return dst
}

// Helpers
func splitMessage(msg string, limit int) []string {
	var result []string
	for len(msg) > limit {
		cut := strings.LastIndex(msg[:limit], "\n")
		if cut == -1 {
			cut = limit
		}
		result = append(result, strings.TrimSpace(msg[:cut]))
		msg = strings.TrimSpace(msg[cut:])
	}
	if msg != "" {
		result = append(result, msg)
	}
	return result
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&ChatCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
