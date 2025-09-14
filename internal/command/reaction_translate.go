package command

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"server-domme/internal/core"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type TranslateOnReaction struct{}

func (t *TranslateOnReaction) Name() string        { return "translate (reaction)" }
func (t *TranslateOnReaction) Description() string { return "Translate message on flag emoji reaction" }
func (t *TranslateOnReaction) Aliases() []string   { return []string{} }
func (c *TranslateOnReaction) Group() string       { return "translate" }
func (t *TranslateOnReaction) Category() string    { return "📢 Utilities" }
func (r *TranslateOnReaction) RequireAdmin() bool  { return false }
func (r *TranslateOnReaction) RequireDev() bool    { return false }

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
	rc, ok := ctx.(*core.ReactionContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	toLangCode, ok := flags[rc.Reaction.Emoji.Name]
	if !ok {
		return nil
	}

	msg, err := rc.Session.ChannelMessage(rc.Reaction.ChannelID, rc.Reaction.MessageID)
	if err != nil || msg.Author.Bot || msg.Content == "" {
		return nil
	}

	translated, detectedLang, err := googleTranslate(msg.Content, toLangCode)
	if err != nil {
		return nil
	}

	if detectedLang == toLangCode {
		return nil
	}

	fromFlag := "🌐"
	for flag, code := range flags {
		if code == detectedLang {
			fromFlag = flag
			break
		}
	}
	toFlag := rc.Reaction.Emoji.Name

	link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", rc.Reaction.GuildID, rc.Reaction.ChannelID, rc.Reaction.MessageID)

	dm, err := rc.Session.UserChannelCreate(rc.Reaction.UserID)
	if err != nil {
		return nil
	}

	content := fmt.Sprintf("%s → %s\n%s\n\n%s", fromFlag, toFlag, translated, link)
	_, _ = rc.Session.ChannelMessageSend(dm.ID, content)

	perms, err := rc.Session.State.UserChannelPermissions(rc.Session.State.User.ID, rc.Reaction.ChannelID)
	if err != nil {
		return nil
	}
	if perms&discordgo.PermissionManageMessages == 0 {
		log.Printf("[WARN] No permission to remove reaction in channel, skipping translation %s\n", rc.Reaction.ChannelID)
		return nil
	}

	return rc.Session.MessageReactionRemove(rc.Reaction.ChannelID, rc.Reaction.MessageID, rc.Reaction.Emoji.Name, rc.Reaction.UserID)
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
	core.RegisterCommand(
		core.WithGroupAccessCheck()(
			core.WithGuildOnly(
				&TranslateOnReaction{},
			),
		),
	)
}
