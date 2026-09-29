package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/events"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/llm"
)

// RouterGroup is the switchboard name for the mention router. Disabling it
// stops all @bot replies without touching slash commands.
const RouterGroup = "llm"

// onMessageCreate reacts only to explicit addresses: a mention of the bot.
// No ambient listening, no memory, no follow-ups. Anything else returns
// without a call, a log line, or a reply.
func (b *Bot) onMessageCreate(e *events.MessageCreate) {
	if e == nil {
		return
	}
	msg := e.Message
	if msg.Author.Bot || msg.WebhookID != nil {
		return
	}
	if e.GuildID == nil {
		return
	}
	guildID := e.GuildID.String()
	if b.isGuildBlacklisted(guildID) {
		return
	}

	self, ok := e.Client().Caches.SelfUser()
	if !ok {
		return
	}
	selfID := self.ID.String()
	mentioned := false
	for _, u := range msg.Mentions {
		if u.ID.String() == selfID {
			mentioned = true
			break
		}
	}
	if !mentioned {
		// Fallback for clients that do not populate mentions: raw token.
		if !strings.Contains(msg.Content, "<@"+selfID+">") && !strings.Contains(msg.Content, "<@!"+selfID+">") {
			return
		}
	}

	text := llm.StripMention(msg.Content, selfID)
	channelID := e.ChannelID.String()
	messageID := msg.ID.String()
	userID := msg.Author.ID.String()
	username := msg.Author.Username

	// Off the gateway goroutine: the LLM call below blocks and the socket
	// must keep reading while it does.
	go b.handleMention(guildID, channelID, messageID, userID, username, text, reply.NewSessionAPI(e.Client()))
}

func (b *Bot) handleMention(guildID, channelID, messageID, userID, username, text string, api interface {
	SendChannelReply(channelID, replyToID, content string) error
}) {
	log := b.log.With().Str("guild_id", guildID).Str("channel_id", channelID).Str("user_id", userID).Logger()

	if disabled, _ := b.storage.IsGroupDisabled(guildID, RouterGroup); disabled {
		log.Info().Msg("router_group_disabled")
		_ = api.SendChannelReply(channelID, messageID, "My mention replies are disabled on this server.")
		return
	}

	if text == "" {
		_ = api.SendChannelReply(channelID, messageID, "Mention me with a request. Try `summarize this channel`, `what are the rules?`, or `give me a task`.")
		return
	}
	// Cap input before it reaches the model or the fallback.
	if len(text) > 1000 {
		text = text[:1000]
	}

	intent := llm.KeywordFallback(text)
	usedLLM := false
	if llm.Ready(b.cfg) {
		client := llm.NewFromConfig(b.cfg)
		ctx := context.Background()
		if parsed := llm.ParseIntent(ctx, client, text); parsed.Name != llm.IntentUnknown || intent.Name == llm.IntentUnknown {
			intent = parsed
			usedLLM = true
		}
	}

	log.Info().Str("intent", intent.Name).Float64("confidence", intent.Confidence).Bool("llm", usedLLM).Int("text_len", len(text)).Msg("router_dispatch")

	var out string
	switch intent.Name {
	case llm.IntentHelp:
		out = "I route plain speech to bot actions. Try `@me summarize this channel`, `@me what are the server rules?`, or `@me give me a task`. Slash still works: `/summarize`, `/task`, `/help`."
	case llm.IntentSummarize:
		out = "Summaries over mentions land in the next slice. For now use `/purge jobs` context or wait for `/summarize`."
	case llm.IntentKnowledge:
		out = "Server knowledge answers land with the knowledge base slice. For now ask an admin or check pinned messages."
	case llm.IntentTaskAsk:
		out = fmt.Sprintf("Task requests over mentions land next. For now use `/task`. You asked: %s", llm.Sanitize(intent.StringArg("query"), 200))
	default:
		out = fmt.Sprintf("I didn't get that. I handle summaries, server questions, and task requests — or use `/help`. Heard: %s", llm.Sanitize(text, 200))
		_ = api.SendChannelReply(channelID, messageID, out)
		return
	}
	_ = api.SendChannelReply(channelID, messageID, llm.Sanitize(out, 2000))
}
