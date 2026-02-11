package chat

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"server-domme/internal/ai"
	"server-domme/internal/command"
	"server-domme/internal/middleware"

	"github.com/bwmarrin/discordgo"
)

type ChatMessageCommand struct{}

func (c *ChatMessageCommand) Name() string             { return "chat" }
func (c *ChatMessageCommand) Description() string      { return "Mention the bot to chat" }
func (c *ChatMessageCommand) Group() string            { return "chat" }
func (c *ChatMessageCommand) Category() string         { return "💬 Chat" }
func (c *ChatMessageCommand) UserPermissions() []int64 { return []int64{} }

func (c *ChatMessageCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.MessageContext)
	if !ok {
		return nil
	}

	// Ignore own messages
	if context.Event.Author.ID == context.Session.State.User.ID {
		return nil
	}

	// Ignore confessions
	confessID, _ := context.Storage.GetConfessChannel(context.Event.GuildID)
	if confessID != "" && context.Event.ChannelID == confessID {
		return nil
	}
	for _, e := range context.Event.Embeds {
		if strings.Contains(e.Title, "📢 Anonymous Confession") {
			return nil
		}
	}

	session := context.Session
	user := context.Event.Author.Username
	display := context.Event.Author.DisplayName()
	userID := context.Event.Author.ID
	channelID := context.Event.ChannelID
	msg := strings.TrimSpace(context.Event.Content)

	log.Printf("[CHAT] %s (%s) @ %s: %s", user, userID, channelID, msg)

	// Start typing indicator goroutine
	done := make(chan struct{})
	go keepTyping(session, channelID, done)

	// DMs not supported
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

	// Add user message to memory
	convos.add(channelID, "user", fmt.Sprintf("User %s: %s", display, msg))
	history := convos.get(channelID)

	// Load system prompt (guild-specific or fallback)
	cfg := context.Config
	globalPromptPath := "data/chat.prompt.md"
	if cfg != nil && cfg.AIPromtPath != "" {
		globalPromptPath = cfg.AIPromtPath
	}
	localPromptPath := filepath.Join("data", fmt.Sprintf("%s_chat.prompt.md", context.Event.GuildID))

	var chosenPath string
	if _, err := os.Stat(localPromptPath); err == nil {
		chosenPath = localPromptPath
	} else if _, err := os.Stat(globalPromptPath); err == nil {
		chosenPath = globalPromptPath
	} else {
		_, _ = context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, I can’t think properly without my system prompt.", display))
		return fmt.Errorf("no prompt found (local or global)")
	}

	file, err := os.Open(chosenPath)
	if err != nil {
		log.Printf("[ERROR] Failed to open system prompt: %v", err)
		context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, I can’t think properly without my system prompt.", display))
		return err
	}
	defer file.Close()

	promptBytes, _ := io.ReadAll(file)
	systemPrompt := string(promptBytes)

	// Build AI conversation
	messages := []ai.Message{{Role: "system", Content: systemPrompt}}
	for _, m := range history {
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		messages = append(messages, ai.Message{Role: role, Content: m.Content})
	}

	client := ai.DefaultProvider(cfg)

	reply, err := client.Generate(messages)

	if mp, ok := client.(*ai.MultiProvider); ok {
		trace := mp.LastTrace()

		if trace.Engine != "" {
			log.Printf("[AI] provider=%s", trace.Engine)
		}

		for _, e := range trace.Errors {
			log.Printf("[AI] fallback error: %s", e)
		}
	}

	if err != nil {
		log.Printf("[ERROR] AI request failed: %v", err)
		context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, something broke 🤯", display))
		return err
	}

	// Save and send reply
	convos.add(channelID, "assistant", reply)
	log.Printf("[CHAT] AI reply to %s (%s) @ %s: %s", user, userID, channelID, reply)

	for _, chunk := range splitMessage(reply, 2000) {
		if _, err := context.Session.ChannelMessageSend(channelID, chunk); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}

	close(done)

	return nil
}

// ---- Conversation store ----

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

func keepTyping(s *discordgo.Session, channelID string, done <-chan struct{}) {
	_ = s.ChannelTyping(channelID)

	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_ = s.ChannelTyping(channelID)
		}
	}
}

func init() {
	command.RegisterCommand(
		&ChatMessageCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
}
