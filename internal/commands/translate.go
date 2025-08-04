package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

var flagToLang = map[string]string{
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

func init() {
	Register(&Command{
		Sort:              90,
		Name:              "translate reaction",
		Category:          "📢 Utilities",
		Description:       "Translate message on flag reaction",
		DCReactionHandler: handleTranslationReaction,
	})
}

func handleTranslationReaction(ctx *ReactionContext) {
	s := ctx.Session
	r := ctx.Reaction
	channelID := r.ChannelID
	messageID := r.MessageID
	userID := r.UserID
	emoji := r.Emoji.Name

	lang, ok := flagToLang[emoji]
	if !ok {
		return
	}

	msg, err := s.ChannelMessage(channelID, messageID)
	if err != nil || msg.Content == "" || msg.Author.Bot {
		return
	}

	translated, err := TranslateGoogle(msg.Content, lang)
	if err != nil {
		log.Printf("Translation failed: %v", err)
		return
	}

	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		log.Printf("Failed to create DM channel: %v", err)
		return
	}

	_, err = s.ChannelMessageSend(channel.ID, fmt.Sprintf("Translated (%s):\n%s", lang, translated))
	if err != nil {
		log.Printf("Failed to send DM: %v", err)
	}

	_ = s.MessageReactionRemove(channelID, messageID, emoji, userID)
}

func TranslateGoogle(text, targetLang string) (string, error) {
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
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var raw interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("unmarshal error: %w", err)
	}

	arr, ok := raw.([]interface{})
	if !ok || len(arr) < 1 {
		return "", fmt.Errorf("unexpected top-level structure")
	}

	sentences, ok := arr[0].([]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected sentences structure")
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

	return translated.String(), nil
}
