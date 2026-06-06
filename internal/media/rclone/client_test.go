package rclone

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/keshon/server-domme/internal/config"
)

func TestClientPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/version" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "v1.65.0"})
	}))
	defer srv.Close()

	c, err := NewFromConfig(&config.Config{
		MediaRcloneRCURL:  srv.URL,
		MediaRcloneRemote: "crypt-media",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFileRemotePath(t *testing.T) {
	tests := []struct {
		name     string
		guildID  string
		category string
		itemPath string
		itemName string
		want     string
	}{
		{
			name:     "full path from rclone",
			guildID:  "guild1",
			category: "random",
			itemPath: "guild1/random/a.jpg",
			itemName: "a.jpg",
			want:     "guild1/random/a.jpg",
		},
		{
			name:     "filename only with category",
			guildID:  "guild1",
			category: "random",
			itemPath: "a.jpg",
			itemName: "a.jpg",
			want:     "guild1/random/a.jpg",
		},
		{
			name:     "recurse relative path",
			guildID:  "guild1",
			category: "",
			itemPath: "memes/a.jpg",
			itemName: "a.jpg",
			want:     "guild1/memes/a.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fileRemotePath(tt.guildID, tt.category, tt.itemPath, tt.itemName)
			if got != tt.want {
				t.Fatalf("fileRemotePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientList(t *testing.T) {
	tests := []struct {
		name         string
		guildID      string
		category     string
		listRemote   string
		items        []map[string]any
		wantPath     string
		wantCategory string
	}{
		{
			name:       "filename only",
			guildID:    "guild1",
			category:   "memes",
			listRemote: "guild1/memes",
			items: []map[string]any{
				{"Path": "cat.jpg", "Name": "cat.jpg", "IsDir": false},
			},
			wantPath:     "guild1/memes/cat.jpg",
			wantCategory: "memes",
		},
		{
			name:       "full path from rclone",
			guildID:    "guild1",
			category:   "random",
			listRemote: "guild1/random",
			items: []map[string]any{
				{"Path": "guild1/random/a.jpg", "Name": "a.jpg", "IsDir": false},
			},
			wantPath:     "guild1/random/a.jpg",
			wantCategory: "random",
		},
		{
			name:       "recurse guild-wide",
			guildID:    "guild1",
			category:   "",
			listRemote: "guild1",
			items: []map[string]any{
				{"Path": "memes/a.jpg", "Name": "a.jpg", "IsDir": false},
			},
			wantPath:     "guild1/memes/a.jpg",
			wantCategory: "memes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/operations/list" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["fs"] != "crypt-media:" {
					t.Fatalf("unexpected fs: %v", body["fs"])
				}
				if body["remote"] != tt.listRemote {
					t.Fatalf("unexpected remote: %v", body["remote"])
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"list": tt.items})
			}))
			defer srv.Close()

			c, err := NewFromConfig(&config.Config{
				MediaRcloneRCURL:  srv.URL,
				MediaRcloneRemote: "crypt-media",
			})
			if err != nil {
				t.Fatal(err)
			}

			files, err := c.List(context.Background(), tt.guildID, tt.category)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 1 {
				t.Fatalf("expected 1 file, got %d", len(files))
			}
			if files[0].Path != tt.wantPath {
				t.Fatalf("unexpected path: %q", files[0].Path)
			}
			if files[0].Category != tt.wantCategory {
				t.Fatalf("unexpected category: %q", files[0].Category)
			}
		})
	}
}

func TestClientReadUsesListPath(t *testing.T) {
	const wantSrc = "guild1/random/a.jpg"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/operations/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"list": []map[string]any{
					{"Path": wantSrc, "Name": "a.jpg", "IsDir": false},
				},
			})
		case "/operations/copyfile":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["srcFs"] != "crypt-media:" {
				t.Fatalf("unexpected srcFs: %v", body["srcFs"])
			}
			if body["srcRemote"] != wantSrc {
				t.Fatalf("unexpected srcRemote: %v", body["srcRemote"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := NewFromConfig(&config.Config{
		MediaRcloneRCURL:  srv.URL,
		MediaRcloneRemote: "crypt-media",
	})
	if err != nil {
		t.Fatal(err)
	}

	files, err := c.List(context.Background(), "guild1", "random")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != wantSrc {
		t.Fatalf("unexpected list path: %q", files[0].Path)
	}

	rc, err := c.Read(context.Background(), files[0].Path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			t.Fatalf("read should not fail with not found for path %q: %v", files[0].Path, err)
		}
		// copyfile mock returns empty success; temp file may be empty but must open.
		t.Fatal(err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	_ = data
}
