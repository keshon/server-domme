// /internal/sources/youtube/resolver.go
package youtube

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

var (
	videoPattern     = regexp.MustCompile(`"url":"/watch\?v=([a-zA-Z0-9_-]+)(?:\\u0026list=([a-zA-Z0-9_-]+))?[^"]*`)
	mixListPattern   = regexp.MustCompile(`/watch\?v=([^&]+)&list=([^&]+)`)
	watchURLPattern  = regexp.MustCompile(`"url":"/watch\?v=([a-zA-Z0-9_-]{11})`)
	ErrNoVideoMatch  = errors.New("no video found for the given title")
	ErrEmptyPlaylist = errors.New("no video URLs found in the playlist")
)

type YouTubeResolver struct {
	BaseURL string
	Client  *http.Client
}

func NewYouTubeResolver() *YouTubeResolver {
	return &YouTubeResolver{
		BaseURL: "https://www.youtube.com",
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (r *YouTubeResolver) SearchFirstVideoURL(query string) (string, error) {
	searchURL := fmt.Sprintf("%s/results?search_query=%s", r.BaseURL, url.QueryEscape(query))

	resp, err := r.Client.Get(searchURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("YouTube search failed with status code %v", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	matches := videoPattern.FindStringSubmatch(string(body))
	if len(matches) > 1 {
		videoID := matches[1]
		listID := matches[2]

		resultURL := fmt.Sprintf("%s/watch?v=%s", r.BaseURL, videoID)
		if listID != "" {
			resultURL += "&list=" + listID
		}
		return resultURL, nil
	}

	return "", ErrNoVideoMatch
}

func (r *YouTubeResolver) ExtractPlaylistVideos(mixURL string) ([]string, error) {
	resp, err := r.Client.Get(mixURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YouTube playlist fetch failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	matches := watchURLPattern.FindAllStringSubmatch(string(body), -1)
	var urls []string
	for _, m := range matches {
		if len(m) > 1 {
			urls = append(urls, fmt.Sprintf("https://www.youtube.com/watch?v=%s", m[1]))
		}
	}

	if len(urls) == 0 {
		return nil, ErrEmptyPlaylist
	}

	return r.removeDuplicates(urls), nil
}

func (r *YouTubeResolver) removeDuplicates(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	var result []string
	for _, u := range input {
		if _, exists := seen[u]; !exists {
			seen[u] = struct{}{}
			result = append(result, u)
		}
	}
	return result
}
