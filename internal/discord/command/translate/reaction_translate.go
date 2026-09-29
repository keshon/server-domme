package translate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

type TranslateOnReaction struct{}

func (t *TranslateOnReaction) Name() string        { return "translate" }
func (t *TranslateOnReaction) Description() string { return "Translate messages with a flag reaction" }
func (c *TranslateOnReaction) Group() string       { return "translate" }
func (t *TranslateOnReaction) Category() string    { return "📢 Utilities" }
func (t *TranslateOnReaction) UserPermissions() []int64 {
	return []int64{}
}

func (t *TranslateOnReaction) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        t.Name(),
		Description: t.Description(),
	}
}

// Run explains the command: translation happens through flag reactions, not
// through this invocation.
func (t *TranslateOnReaction) Run(ctx *adapter.SlashInteractionContext) error {
	return ctx.RespondEphemeral(&adapter.Embed{
		Description: "React to a message with a flag (🇬🇧, 🇷🇺, …) and I'll DM you the translation.\nAn admin enables channels with `/settings translate channel-add`.",
		Color:       reply.EmbedColor,
	})
}

var flags = map[string]string{
	"🇷🇺": "ru",
	"🇬🇧": "en",
	"🇺🇸": "en",
	"🇫🇷": "fr",
	"🇩🇪": "de",
	"🇪🇸": "es",
	"🇮🇹": "it",
	"🇯🇵": "ja",
	"🇨🇳": "zh",
}

func (t *TranslateOnReaction) React(ctx *adapter.ReactionContext) error {
	channels, err := ctx.Storage.GetTranslateChannels(ctx.GuildID())
	if err != nil {
		return nil // silently ignore if we can't fetch channels
	}

	found := false
	for _, ch := range channels {
		if ch == ctx.ChannelID() {
			found = true
			break
		}
	}

	if !found {
		return nil // channel not configured for translation reactions
	}

	// Determine target language from flag
	toLangCode, ok := flags[ctx.Emoji]
	if !ok {
		return nil
	}

	// Fetch message
	msg, err := ctx.API.ChannelMessage(ctx.ChannelID(), ctx.MessageID)
	if err != nil || msg.Content == "" {
		return nil
	}

	// Translate
	translated, detectedLang, err := googleTranslate(msg.Content, toLangCode)
	if err != nil || detectedLang == toLangCode {
		return nil
	}

	// Map detected language to flag
	fromFlag := "🌐"
	for flag, code := range flags {
		if code == detectedLang {
			fromFlag = flag
			break
		}
	}

	link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", ctx.GuildID(), ctx.ChannelID(), ctx.MessageID)

	content := fmt.Sprintf("%s → %s\n%s\n\n%s", fromFlag, ctx.Emoji, translated, link)
	if err := ctx.API.SendDirectMessage(ctx.UserID(), content); err != nil {
		ctx.AppLog.Warn().Str("user_id", ctx.UserID()).Err(err).Msg("translate_dm_failed")
		// Leave the reaction in place: it is the only cue the user gets that
		// nothing arrived, and removing it would look like the work succeeded.
		return nil
	}

	// Remove reaction if we have permissions
	if ctx.API.CheckBotPermissions(ctx.ChannelID()) {
		if err := ctx.API.RemoveReaction(ctx.ChannelID(), ctx.MessageID, ctx.Emoji, ctx.UserID()); err != nil {
			ctx.AppLog.Debug().Str("channel_id", ctx.ChannelID()).Err(err).Msg("translate_reaction_remove_failed")
		}
	}

	return nil
}

func googleTranslate(text, targetLang string) (string, string, error) {
	endpoint := "https://translate.googleapis.com/translate_a/single"
	params := url.Values{}
	params.Set("client", "gtx")
	params.Set("sl", "auto")
	params.Set("tl", targetLang)
	params.Set("dt", "t")
	params.Set("q", text)

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	var raw interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", "", fmt.Errorf("translate: unmarshal: %w", err)
	}

	arr, ok := raw.([]interface{})
	if !ok || len(arr) < 2 {
		return "", "", fmt.Errorf("translate: unexpected top-level structure")
	}

	// arr[0] — translated sentences
	// arr[2] — source language
	detectedLang := "auto"
	if arr[2] != nil {
		if detectedStr, ok := arr[2].(string); ok {
			detectedLang = detectedStr
		}
	}

	sentences, ok := arr[0].([]interface{})
	if !ok {
		return "", "", fmt.Errorf("translate: unexpected sentences structure")
	}

	var translated strings.Builder
	for _, part := range sentences {
		pair, ok := part.([]interface{})
		if !ok || len(pair) < 1 {
			continue
		}
		str, ok := pair[0].(string)
		if ok {
			translated.WriteString(str)
		}
	}

	return translated.String(), detectedLang, nil
}
