package summarize

import (
	"context"
	"fmt"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/llm"
)

// Limits: one page of history, bounded transcript into the model.
const (
	defaultLimit = 50
	maxLimit     = 100
	maxTranscriptChars = 12000
)

// Command summarizes recent channel history. Group llm shares the router's
// switchboard: disabling llm stops both the slash and the mention path.
type Command struct{}

func (c *Command) Name() string        { return "summarize" }
func (c *Command) Description() string { return "Summarize recent messages in this channel" }
func (c *Command) Group() string       { return "llm" }
func (c *Command) Category() string    { return "📢 Utilities" }
func (c *Command) UserPermissions() []int64 {
	return []int64{}
}

func floatPtr(f float64) *float64 { return &f }

func (c *Command) SlashDefinition() *adapter.SlashCommand {
	min := 10.0
	max := float64(maxLimit)
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionInteger,
				Name:        "limit",
				Description: "How many recent messages to include",
				MinValue:    &min,
				MaxValue:    max,
			},
		},
	}
}

func (c *Command) Run(ctx *adapter.SlashInteractionContext) error {
	limit := int(ctx.IntOption("limit"))
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 10 {
		limit = 10
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	if !llm.Ready(ctx.Config) {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Summaries need an LLM backend. Set `LLM_ENABLED=true` with `LLM_BASE_URL` and `LLM_MODEL` first.",
			Color:       reply.EmbedColor,
		})
	}

	if err := ctx.DeferEphemeral(); err != nil {
		ctx.AppLog.Error().Err(err).Msg("summarize_ack_failed")
		return nil
	}

	text, count, err := Run(context.Background(), ctx.API, ctx.ChannelID(), limit, llm.NewFromConfig(ctx.Config))
	if err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to summarize: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}
	return ctx.FollowupEphemeral(&adapter.Embed{
		Title:       fmt.Sprintf("Summary — last %d messages", count),
		Description: text,
		Color:       reply.EmbedColor,
	})
}

// Run fetches history, builds a transcript, and returns sanitized summary
// text plus the message count actually summarized. Shared by the slash
// command and the mention router so both read the same history the same way.
func Run(ctx context.Context, api adapter.SessionAPI, channelID string, limit int, client *llm.Client) (string, int, error) {
	if api == nil {
		return "", 0, fmt.Errorf("summarize: no Discord session")
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	msgs, err := api.ChannelMessages(channelID, "", limit)
	if err != nil {
		return "", 0, fmt.Errorf("summarize: listing messages: %w", err)
	}
	transcript, count := BuildTranscript(msgs)
	if count == 0 {
		return "", 0, fmt.Errorf("summarize: nothing readable in the last %d messages", limit)
	}
	raw, err := client.Complete(ctx, Prompt(transcript), floatPtr(0.4))
	if err != nil {
		return "", 0, err
	}
	return llm.Sanitize(raw, 4000), count, nil
}

// BuildTranscript renders newest-first history oldest-first, one line per
// message, skipping empty and bot messages. Returns text and readable count.
func BuildTranscript(msgs []adapter.ListedMessage) (string, int) {
	var lines []string
	count := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		text := strings.TrimSpace(m.Content)
		if text == "" || m.Bot {
			continue
		}
		if len(text) > 500 {
			text = text[:500] + "…"
		}
		name := m.AuthorName
		if name == "" {
			name = "someone"
		}
		lines = append(lines, name+": "+strings.Join(strings.Fields(text), " "))
		count++
	}
	out := strings.Join(lines, "\n")
	if len(out) > maxTranscriptChars {
		out = out[len(out)-maxTranscriptChars:]
		if idx := strings.Index(out, "\n"); idx >= 0 {
			out = out[idx+1:]
		}
	}
	return out, count
}

// Prompt wraps the transcript in a neutral summarizer brief. Facts in,
// no persona, bullets out.
func Prompt(transcript string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: "Summarize this Discord channel excerpt for someone who missed it. 5-8 bullets: topics, decisions, open questions. Names as written. No preamble, no advice, no invented facts. If nothing substantive, say so in one line."},
		{Role: "user", Content: transcript},
	}
}
