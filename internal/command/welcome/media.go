package welcome

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Gifs are posted as files, not links. A link to a gif page posted with the
// welcome shows its address above the picture; Discord hides it only when a
// message is nothing but a link from a site it knows as a gif source. So the
// page a gif was added from is read once for the file behind it — the
// og:image and friends every gif site puts in its pages for link previews —
// and each welcome attaches that file, the way a person posting a gif would.
// Anything that fails falls back to the link: a welcome never goes out
// without its gif because a site changed.

// gifClient fetches gif pages and files. The timeout bounds a slow site;
// both calls happen after the interaction was acknowledged.
var gifClient = &http.Client{Timeout: 15 * time.Second}

// Limits on what is read. A page is read only as far as its head, give or
// take; a file is refused past what Discord takes from a bot without boosts.
const (
	maxPageBytes = 1 << 20
	maxGifBytes  = 10 << 20
)

// errNoGif is a page that names no picture to post.
var errNoGif = errors.New("welcome: the page names no gif")

// metaTag and metaAttr read a page's meta tags, whatever order their
// attributes come in.
var (
	metaTag  = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	metaAttr = regexp.MustCompile(`(?is)([a-z][a-z0-9:_-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// previewKeys are the meta properties a gif site's page names its picture
// in, most useful first.
var previewKeys = []string{
	"og:image:secure_url", "og:image", "og:image:url", "twitter:image", "twitter:image:src",
	"og:video:secure_url", "og:video", "og:video:url",
}

// resolveGif finds the file behind a gif link: the link itself when it
// already is one, otherwise the picture its page names for previews — a
// .gif first, since that is what animates when posted as a file.
func resolveGif(ctx context.Context, c *http.Client, link string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", fmt.Errorf("welcome: gif page: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("welcome: gif page: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("welcome: gif page: %s", resp.Status)
	}
	if isMedia(resp.Header.Get("Content-Type")) {
		return link, nil
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return "", fmt.Errorf("welcome: gif page: %w", err)
	}
	base := resp.Request.URL

	found := map[string]string{}
	for _, tag := range metaTag.FindAll(page, -1) {
		attrs := map[string]string{}
		for _, a := range metaAttr.FindAllSubmatch(tag, -1) {
			v := string(a[2]) + string(a[3])
			attrs[strings.ToLower(string(a[1]))] = html.UnescapeString(v)
		}
		key := attrs["property"]
		if key == "" {
			key = attrs["name"]
		}
		key = strings.ToLower(key)
		if _, seen := found[key]; !seen && attrs["content"] != "" {
			found[key] = attrs["content"]
		}
	}
	var candidates []string
	for _, k := range previewKeys {
		if v := found[k]; v != "" {
			if u, err := base.Parse(v); err == nil && (u.Scheme == "https" || u.Scheme == "http") {
				candidates = append(candidates, u.String())
			}
		}
	}
	for _, c := range candidates {
		if strings.EqualFold(path.Ext(pathOf(c)), ".gif") {
			return c, nil
		}
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return "", errNoGif
}

// fetchGif downloads a gif file to attach.
func fetchGif(ctx context.Context, c *http.Client, media string) (*discordgo.File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, media, nil)
	if err != nil {
		return nil, fmt.Errorf("welcome: gif file: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("welcome: gif file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("welcome: gif file: %s", resp.Status)
	}
	ct := resp.Header.Get("Content-Type")
	if !isMedia(ct) {
		return nil, fmt.Errorf("welcome: gif file is %q, not a picture", ct)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGifBytes+1))
	if err != nil {
		return nil, fmt.Errorf("welcome: gif file: %w", err)
	}
	if len(body) > maxGifBytes {
		return nil, fmt.Errorf("welcome: gif file is over %d MB", maxGifBytes>>20)
	}
	return &discordgo.File{Name: gifName(media, ct), ContentType: ct, Reader: strings.NewReader(string(body))}, nil
}

// isMedia reports whether a content type is a picture or a video.
func isMedia(contentType string) bool {
	t, _, err := mime.ParseMediaType(contentType)
	return err == nil && (strings.HasPrefix(t, "image/") || strings.HasPrefix(t, "video/"))
}

// gifName is the attachment's file name: "welcome" and the file's own
// extension, or one for its type.
func gifName(media, contentType string) string {
	ext := strings.ToLower(path.Ext(pathOf(media)))
	if ext == "" || len(ext) > 5 {
		ext = ".gif"
		if t, _, err := mime.ParseMediaType(contentType); err == nil {
			if exts, _ := mime.ExtensionsByType(t); len(exts) > 0 {
				ext = exts[0]
			}
		}
	}
	return "welcome" + ext
}

// pathOf is a URL's path, or "" when it is not one.
func pathOf(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return u.Path
}
