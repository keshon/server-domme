package chat

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"server-domme/internal/ai"
	"server-domme/internal/command"
	"server-domme/internal/config"
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

	if context.Event.Author.ID == context.Session.State.User.ID {
		return nil
	}

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
	display := context.Event.Author.DisplayName()
	userID := context.Event.Author.ID
	channelID := context.Event.ChannelID
	guildID := context.Event.GuildID
	msg := strings.TrimSpace(context.Event.Content)

	log.Printf("[CHAT] %s (%s) @ %s: %s", context.Event.Author.Username, userID, channelID, msg)

	done := make(chan struct{})
	go keepTyping(session, channelID, done)

	if guildID == "" {
		_, err := context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, I don't chat in DMs. Speak on a server channel.", display))
		return err
	}

	if msg == "" {
		_, err := context.Session.ChannelMessageSend(channelID,
			fmt.Sprintf("%s, speak or be silent forever.", display))
		return err
	}

	var messages []ai.Message
	if context.BuildMessagesForReactiveChat != nil {
		msgs, err := context.BuildMessagesForReactiveChat(guildID, channelID)
		if err != nil {
			log.Printf("[CHAT] mind context failed: %v, using fallback", err)
			messages = c.fallbackMessages(guildID, display, msg, context.Config)
		} else {
			messages = msgs
		}
	} else {
		messages = c.fallbackMessages(guildID, display, msg, context.Config)
	}

	logChatPrompt(guildID, channelID, messages)

	client := ai.DefaultProvider(context.Config)
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

	log.Printf("[CHAT] reply to %s @ %s: %s", display, channelID, truncateLog(reply, 120))

	for _, chunk := range splitMessage(reply, 2000) {
		if _, err := context.Session.ChannelMessageSend(channelID, chunk); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}

	if context.RecordAssistantReply != nil {
		context.RecordAssistantReply(guildID, channelID, reply)
	}

	close(done)
	return nil
}

// fallbackMessages builds [system, user] when mind is not available (identity file + current message only).
func (c *ChatMessageCommand) fallbackMessages(guildID, display, msg string, cfg *config.Config) []ai.Message {
	identityPath := "data/mind/core/identity.md"
	if cfg != nil && cfg.AIPromtPath != "" {
		identityPath = cfg.AIPromtPath
	}
	localPath := filepath.Join("data", fmt.Sprintf("%s_chat.prompt.md", guildID))
	var path string
	if _, err := os.Stat(localPath); err == nil {
		path = localPath
	} else if _, err := os.Stat(identityPath); err == nil {
		path = identityPath
	} else {
		return []ai.Message{
			{Role: "system", Content: "You are a helpful character."},
			{Role: "user", Content: fmt.Sprintf("User %s: %s", display, msg)},
		}
	}
	body, _ := os.ReadFile(path)
	system := string(body)
	if system == "" {
		system = "You are a helpful character."
	}
	userContent := fmt.Sprintf("User %s: %s", display, msg)
	return []ai.Message{
		{Role: "system", Content: strings.TrimSpace(system)},
		{Role: "user", Content: userContent},
	}
}

func logChatPrompt(guildID, channelID string, messages []ai.Message) {
	log.Printf("[CHAT] prompt guild=%s channel=%s messages=%d", guildID, channelID, len(messages))
	if len(messages) == 0 {
		return
	}
	sysLen := len(messages[0].Content)
	preview := messages[0].Content
	if len(preview) > 400 {
		preview = preview[:400] + "..."
	}
	log.Printf("[CHAT] system_len=%d preview: %s", sysLen, preview)
	for i := 1; i < len(messages); i++ {
		m := messages[i]
		p := m.Content
		if len(p) > 200 {
			p = p[:200] + "..."
		}
		log.Printf("[CHAT] msg[%d] role=%s len=%d: %s", i, m.Role, len(m.Content), p)
	}
}

func truncateLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
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
