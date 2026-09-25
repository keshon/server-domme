package welcome

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
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
<title>join us</title></head><body>…</body></html>`)
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
		case "/plain":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<html><head><title>nothing here</title></head></html>")
		case "/huge.gif":
			w.Header().Set("Content-Type", "image/gif")
			_, _ = w.Write(make([]byte, maxGifBytes+1))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A page is read for the file behind it, a .gif before anything else, with
// a relative address made whole and entities undone.
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
	if err != nil || f.Name != "welcome.gif" || f.ContentType != "image/gif" {
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
	rec := &recorder{}
	s := &discordgo.Session{State: discordgo.NewState(), Client: &http.Client{Transport: rec}, Ratelimiter: discordgo.NewRatelimiter()}
	p := &part{label: "Welcome", channelID: "c", content: "please welcome <@1>",
		gif:  "https://klipy.com/gifs/shushes-come-join-the-call",
		file: &discordgo.File{Name: "welcome.gif", ContentType: "image/gif", Reader: strings.NewReader("GIF89a")}}
	p.send(s, "g", "1")
	if len(rec.bodies) != 1 {
		t.Fatalf("sent %v", rec.paths)
	}
	body := rec.bodies[0]
	if !strings.Contains(body, `filename="welcome.gif"`) || strings.Contains(body, "klipy.com") {
		t.Errorf("sent %q", body)
	}
}

// refusing stands in for Discord refusing the first upload for a missing
// permission, and accepting what comes next.
type refusing struct {
	recorder
	refused bool
}

func (r *refusing) RoundTrip(req *http.Request) (*http.Response, error) {
	if !r.refused && req.Method == http.MethodPost {
		r.refused = true
		_, _ = r.recorder.RoundTrip(req)
		return &http.Response{StatusCode: http.StatusForbidden, Request: req,
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   io.NopCloser(strings.NewReader(`{"message": "Missing Permissions", "code": 50013}`))}, nil
	}
	return r.recorder.RoundTrip(req)
}

// Attaching a file needs Attach Files, which View Channel and Send Messages
// do not carry — an administrator ticks those two and the welcome is still
// refused. The words matter more than the gif: it goes out with the link.
func TestAWelcomeRefusedTheAttachmentGoesOutWithTheLink(t *testing.T) {
	rec := &refusing{}
	s := &discordgo.Session{State: discordgo.NewState(), Client: &http.Client{Transport: rec}, Ratelimiter: discordgo.NewRatelimiter()}
	gif := "https://klipy.com/gifs/feel-better-33"
	p := &part{label: "Welcome", channelID: "c", content: "please welcome <@1>", gif: gif,
		file: &discordgo.File{Name: "welcome.gif", ContentType: "image/gif", Reader: strings.NewReader("GIF89a")}}

	if !p.send(s, "g", "1") {
		t.Fatalf("nothing went out: %s", p.problem)
	}
	if len(rec.bodies) != 2 {
		t.Fatalf("sent %v", rec.paths)
	}
	second := rec.bodies[1]
	if !strings.Contains(second, gif) || strings.Contains(second, "filename=") {
		t.Errorf("the second try sent %q", second)
	}
	if !strings.Contains(p.report(), "cannot attach files") {
		t.Errorf("the report does not say why: %s", p.report())
	}
}

// Any other refusal is still a failure, and says the channel rather than a
// JSON body.
func TestAWelcomeRefusedOutrightSaysWhere(t *testing.T) {
	rec := &refusing{}
	s := &discordgo.Session{State: discordgo.NewState(), Client: &http.Client{Transport: rec}, Ratelimiter: discordgo.NewRatelimiter()}
	p := &part{label: "Welcome", channelID: "c", content: "please welcome <@1>"}
	if p.send(s, "g", "1") {
		t.Fatal("it claimed to post")
	}
	if !strings.Contains(p.problem, "<#c>") || strings.Contains(p.problem, "50013") {
		t.Errorf("problem is %q", p.problem)
	}
}
