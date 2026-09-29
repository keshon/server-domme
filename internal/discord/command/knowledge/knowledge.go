package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/llm"
	st "github.com/keshon/server-domme/internal/storage"
)

// Command answers plain-speech server questions from admin-curated docs.
// Group llm shares the router's switchboard.
type Command struct{}

func (c *Command) Name() string        { return "knowledge" }
func (c *Command) Description() string { return "Ask server questions answered from curated notes" }
func (c *Command) Group() string       { return "llm" }
func (c *Command) Category() string    { return "📢 Utilities" }
func (c *Command) UserPermissions() []int64 {
	return []int64{}
}

func (c *Command) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionString,
				Name:        "query",
				Description: "What do you want to know about this server?",
				Required:    true,
			},
		},
	}
}

func (c *Command) Run(ctx *adapter.SlashInteractionContext) error {
	query := strings.TrimSpace(ctx.StringOption("query"))
	if query == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Ask something, like `what are the rules on spoilers?`.",
			Color:       reply.EmbedColor,
		})
	}
	if !llm.Ready(ctx.Config) {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Knowledge answers need an LLM backend. Set `LLM_ENABLED=true` with `LLM_BASE_URL` and `LLM_MODEL` first.",
			Color:       reply.EmbedColor,
		})
	}
	if err := ctx.DeferEphemeral(); err != nil {
		ctx.AppLog.Error().Err(err).Msg("knowledge_ack_failed")
		return nil
	}
	text, err := Run(context.Background(), ctx.Storage, ctx.GuildID(), query, llm.NewFromConfig(ctx.Config))
	if err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to answer: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}
	return ctx.FollowupEphemeral(&adapter.Embed{
		Title:       "Server Knowledge",
		Description: text,
		Color:       reply.EmbedColor,
	})
}

// Run retrieves docs and answers with citations. Shared by slash and router.
func Run(ctx context.Context, store *st.Storage, guildID, query string, client *llm.Client) (string, error) {
	if store == nil {
		return "", fmt.Errorf("knowledge: no storage")
	}
	docs := store.ListKnowledgeDocs(guildID)
	if len(docs) == 0 {
		return "", fmt.Errorf("knowledge: no notes yet. An admin can add some with `/settings knowledge add`.")
	}
	top := Retrieve(docs, query, 5)
	if len(top) == 0 {
		return "", fmt.Errorf("knowledge: nothing relevant. Try different words or ask an admin.")
	}
	raw, err := client.Complete(ctx, Prompt(top, query), ptrTemp(0.4))
	if err != nil {
		return "", err
	}
	return llm.Sanitize(raw, 4000), nil
}

// Retrieve ranks docs by token overlap: title hits weigh triple. Returns at
// most n docs with a positive score, best first.
func Retrieve(docs []st.KnowledgeDoc, query string, n int) []st.KnowledgeDoc {
	tokens := tokensOf(query)
	if len(tokens) == 0 {
		return nil
	}
	type scored struct {
		doc   st.KnowledgeDoc
		score int
	}
	var ranked []scored
	for _, d := range docs {
		title := strings.ToLower(d.Title)
		body := strings.ToLower(d.Body)
		s := 0
		for _, tok := range tokens {
			if strings.Contains(title, tok) {
				s += 3
			}
			if strings.Contains(body, tok) {
				s += 1
			}
		}
		if s > 0 {
			ranked = append(ranked, scored{doc: d, score: s})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].doc.Slug < ranked[j].doc.Slug
	})
	if n <= 0 || n > len(ranked) {
		n = len(ranked)
	}
	out := make([]st.KnowledgeDoc, 0, n)
	for i := 0; i < n && i < len(ranked); i++ {
		out = append(out, ranked[i].doc)
	}
	return out
}

// Prompt numbers the sources so the answer can cite [1][2]. Refuse rather
// than invent when nothing fits.
func Prompt(docs []st.KnowledgeDoc, query string) []llm.Message {
	var b strings.Builder
	for i, d := range docs {
		body := d.Body
		if len(body) > 1500 {
			body = body[:1500] + "…"
		}
		fmt.Fprintf(&b, "[%d] %s\n%s\n\n", i+1, d.Title, body)
	}
	return []llm.Message{
		{Role: "system", Content: "Answer the question using only the numbered notes below. Cite sources like [1][2]. Keep it short. If none of the notes answer it, say so in one line and do not invent."},
		{Role: "user", Content: "Notes:\n" + b.String() + "Question: " + query},
	}
}

func tokensOf(s string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,!?;:\"'()[]{}")
		if len(w) < 3 {
			continue
		}
		out = append(out, w)
		if len(out) == 12 {
			break
		}
	}
	return out
}

func ptrTemp(f float64) *float64 { return &f }
