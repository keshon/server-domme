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

// Handle messages mentioning the bot
func (c *ChatCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.MessageContext)
	if !ok {
		return nil
	}

	// Ignore own messages
	if context.Event.Author.ID == context.Session.State.User.ID {
		return nil
	}

	// Ignore confession channel/messages
	confessID, _ := context.Storage.GetSpecialChannel(context.Event.GuildID, "confession")
	if confessID != "" && context.Event.ChannelID == confessID {
		return nil
	}
	for _, e := range context.Event.Embeds {
		if strings.Contains(e.Title, "📢 Anonymous Confession") {
			return nil
		}
	}

	// Collect basic info
	user := context.Event.Author.Username
	display := context.Event.Author.DisplayName()
	userID := context.Event.Author.ID
	channelID := context.Event.ChannelID
	msg := strings.TrimSpace(context.Event.Content)

	log.Printf("[CHAT] %s (%s) @ %s: %s", user, userID, channelID, msg)

	// DM check
	if context.Event.GuildID == "" {
		_, err := context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, I don't chat in DMs. Speak on a server channel.", display))
		return err
	}

	if msg == "" {
		_, err := context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, speak or be silent forever.", display))
		return err
	}

	// Add user message to conversation history
	convos.add(channelID, "user", fmt.Sprintf("User %s: %s", display, msg))
	history := convos.get(channelID)

	// Load system prompt
	cfg := config.New()
	file, err := os.Open(cfg.AIPromtPath)
	if err != nil {
		log.Printf("[ERROR] Missing system prompt: %v", err)
		context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, I can't think properly without my system prompt.", display))
		return err
	}
	defer file.Close()

	promptBytes, _ := io.ReadAll(file)
	systemPrompt := string(promptBytes)

	// Build AI messages
	messages := []ai.Message{{Role: "system", Content: systemPrompt}}
	for _, m := range history {
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		messages = append(messages, ai.Message{Role: role, Content: m.Content})
	}

	// Generate AI reply
	client := ai.DefaultProvider()
	reply, err := client.Generate(messages)
	if err != nil {
		log.Printf("[ERROR] AI request failed: %v", err)
		context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, something broke 🤯", display))
		return err
	}

	// Save AI reply
	convos.add(channelID, "assistant", reply)
	log.Printf("[CHAT] AI reply to %s (%s) @ %s: %s", user, userID, channelID, reply)

	// Send reply (split if too long)
	for _, chunk := range splitMessage(reply, 2000) {
		if _, err := context.Session.ChannelMessageSend(channelID, chunk); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

// Conversation store
type convoMsg struct {
	Role, Content string
}

type convoStore struct {
	mu       sync.Mutex
	store    map[string][]convoMsg
	maxMsgs  int
	maxChars int
}

var convos = &convoStore{
	store:    map[string][]convoMsg{},
	maxMsgs:  60,
	maxChars: 25000,
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

// Split long messages into chunks
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
			core.WithAccessControl(),
		),
	)
}
