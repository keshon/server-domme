package welcome

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gifSite serves what a gif site does: a page naming its picture in meta
// tags, the picture itself, a page naming nothing, and a file too big.
func gifSite(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// As klipy does: pages only for the link previewers of chat apps.
		if !strings.Contains(r.UserAgent(), "Discordbot") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/gifs/shushes-come-join-the-call":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<html><head>
<meta content="https://example.invalid/preview.webp" property="og:image">
<meta name="twitter:image" content="/media/join.gif?x=1&amp;y=2">
<title>join us</title></head><body>�?�</body></html>`)
		case "/media/join.gif":
			w.Header().Set("Content-Type", "image/gif")
			_, _ = io.WriteString(w, "GIF89a...")
		case "/gifs/top-gear-29":
			// klipy's own order: the same property twice, .webp first.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<meta property="og:image" content="https://static2.klipy.com/ii/a/mJMm.webp"/>
<meta property="og:image:type" content="image/webp"/>
<meta property="og:image" content="https://static2.klipy.com/ii/a/aFm9.gif"/>
<meta property="og:video:url" content="https://static2.klipy.com/ii/a/sRbQ.mp4"/>`)
		case "/huge.gif":
			w.Header().Set("Content-Type", "image/gif")
			_, _ = io.WriteString(w, strings.Repeat("x", maxGifBytes+2))
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<html><head><title>nothing here</title></head></html>`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAGifPageIsReadForItsFile(t *testing.T) {
	srv := gifSite(t)
	ctx := context.Background()
	got, err := resolveGif(ctx, srv.Client(), srv.URL+"/gifs/shushes-come-join-the-call")
	if err != nil || got != srv.URL+"/media/join.gif?x=1&y=2" {
		t.Errorf("resolved %q, %v", got, err)
	}
	// Two pictures under one property: the .gif, not the first.
	if got, err := resolveGif(ctx, srv.Client(), srv.URL+"/gifs/top-gear-29"); err != nil || got != "https://static2.klipy.com/ii/a/aFm9.gif" {
		t.Errorf("klipy's page resolved to %q, %v", got, err)
	}
	// A link to the file already is the file.
	if got, err := resolveGif(ctx, srv.Client(), srv.URL+"/media/join.gif"); err != nil || got != srv.URL+"/media/join.gif" {
		t.Errorf("a direct link resolved to %q, %v", got, err)
	}
	if _, err := resolveGif(ctx, srv.Client(), srv.URL+"/plain"); err == nil {
		t.Error("a page naming no picture resolved")
	}
}

func TestAGifFileIsFetchedWithinLimits(t *testing.T) {
	srv := gifSite(t)
	ctx := context.Background()
	f, err := fetchGif(ctx, srv.Client(), srv.URL+"/media/join.gif?x=1")
	if err != nil || f.name != "welcome.gif" {
		t.Fatalf("fetched %+v, %v", f, err)
	}
	if _, err := fetchGif(ctx, srv.Client(), srv.URL+"/huge.gif"); err == nil {
		t.Error("a file past Discord's limit was fetched")
	}
	if _, err := fetchGif(ctx, srv.Client(), srv.URL+"/plain"); err == nil {
		t.Error("a page was fetched as a gif")
	}
}

// With the file in hand the welcome carries it, and not the link.
func TestAWelcomeAttachesTheGifInPlaceOfTheLink(t *testing.T) {
	api := newAPIFake()
	ctx := testCtx(testStore(t), api)
	p := &part{ctx: ctx, label: "Welcome", channelID: "c", content: "please welcome <@1>",
		gif:  "https://klipy.com/gifs/shushes-come-join-the-call",
		file: &gifAttachment{name: "welcome.gif", data: []byte("GIF89a")}}
	if !p.send(ctx, "1") {
		t.Fatalf("nothing went out: %s", p.problem)
	}
	post := api.lastPost()
	if post.msg.File == nil || post.msg.FileName != "welcome.gif" {
		t.Errorf("no file attached: %+v", post.msg)
	}
	if strings.Contains(post.msg.Content, "klipy.com") {
		t.Errorf("the link went out beside the file: %q", post.msg.Content)
	}
}

// The first upload is refused for a missing permission; the words matter
// more than the gif, so it goes out with the link.
func TestAWelcomeRefusedTheAttachmentGoesOutWithTheLink(t *testing.T) {
	api := newAPIFake()
	api.postErrs = []error{errors.New(`{"message": "Missing Permissions", "code": 50013}`)}
	ctx := testCtx(testStore(t), api)
	gif := "https://klipy.com/gifs/feel-better-33"
	p := &part{ctx: ctx, label: "Welcome", channelID: "c", content: "please welcome <@1>", gif: gif,
		file: &gifAttachment{name: "welcome.gif", data: []byte("GIF89a")}}

	if !p.send(ctx, "1") {
		t.Fatalf("nothing went out: %s", p.problem)
	}
	post := api.lastPost()
	if post.msg.File != nil {
		t.Error("the second try still carried the file")
	}
	if !strings.Contains(post.msg.Content, gif) {
		t.Errorf("the second try lost the link: %q", post.msg.Content)
	}
	if !strings.Contains(p.report(), "cannot attach files") {
		t.Errorf("the report does not say why: %s", p.report())
	}
}

// Any other refusal is still a failure, and says the channel rather than a
// JSON body.
func TestAWelcomeRefusedOutrightSaysWhere(t *testing.T) {
	api := newAPIFake()
	api.postErrs = []error{errors.New("Missing Permissions")}
	ctx := testCtx(testStore(t), api)
	p := &part{ctx: ctx, label: "Welcome", channelID: "c", content: "please welcome <@1>"}
	if p.send(ctx, "1") {
		t.Fatal("it claimed to post")
	}
	if !strings.Contains(p.problem, "<#c>") || strings.Contains(p.problem, "50013") {
		t.Errorf("problem is %q", p.problem)
	}
}
