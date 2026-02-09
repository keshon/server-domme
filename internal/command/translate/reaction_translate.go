package translate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"server-domme/internal/command"
	"server-domme/internal/middleware"

	"strings"

	"github.com/bwmarrin/discordgo"
)

type TranslateOnReaction struct{}

func (t *TranslateOnReaction) Name() string        { return "translate (reaction)" }
func (t *TranslateOnReaction) Description() string { return "Translate message on flag emoji reaction" }
func (c *TranslateOnReaction) Group() string       { return "translate" }
func (t *TranslateOnReaction) Category() string    { return "📢 Utilities" }
func (t *TranslateOnReaction) UserPermissions() []int64 {
	return []int64{}
}
func (t *TranslateOnReaction) ReactionDefinition() string { return "reaction" }

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

func (t *TranslateOnReaction) Run(ctx interface{}) error {
	context, ok := ctx.(*command.MessageReactionContext)
	if !ok {
		return nil
	}

	s, e, storage := context.Session, context.Event, context.Storage

	// Check if the channel is in the translate reaction list
	channels, err := storage.GetTranslateChannels(e.GuildID)
	if err != nil {
		return nil // silently ignore if we can't fetch channels
	}

	found := false
	for _, ch := range channels {
		if ch == e.ChannelID {
			found = true
			break
		}
	}

	if !found {
		return nil // channel not configured for translation reactions
	}

	// Determine target language from flag
	toLangCode, ok := flags[e.Emoji.Name]
	if !ok {
		return nil
	}

	// Fetch message
	msg, err := s.ChannelMessage(e.ChannelID, e.MessageID)
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
	toFlag := e.Emoji.Name

	link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", e.GuildID, e.ChannelID, e.MessageID)

	// Send DM to user
	dm, err := s.UserChannelCreate(e.UserID)
	if err != nil {
		return nil
	}

	content := fmt.Sprintf("%s → %s\n%s\n\n%s", fromFlag, toFlag, translated, link)
	s.ChannelMessageSend(dm.ID, content)

	// Remove reaction if we have permissions
	perms, err := s.State.UserChannelPermissions(s.State.User.ID, e.ChannelID)
	if err == nil && perms&discordgo.PermissionManageMessages != 0 {
		s.MessageReactionRemove(e.ChannelID, e.MessageID, e.Emoji.Name, e.UserID)
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
		return "", "", fmt.Errorf("unmarshal error: %w", err)
	}

	arr, ok := raw.([]interface{})
	if !ok || len(arr) < 2 {
		return "", "", fmt.Errorf("unexpected top-level structure")
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
		return "", "", fmt.Errorf("unexpected sentences structure")
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

func init() {
	command.RegisterCommand(
		&TranslateOnReaction{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
}
