package rclone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/media"
)

// Client implements media.Store via the rclone Remote Control HTTP API.
type Client struct {
	baseURL    string
	remote     string
	httpClient *http.Client
	user       string
	pass       string
}

// NewFromConfig builds an RC client from bot configuration.
func NewFromConfig(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rclone: config is nil")
	}
	if cfg.MediaRcloneRemote == "" {
		return nil, fmt.Errorf("rclone: MEDIA_RCLONE_REMOTE is required")
	}
	if cfg.MediaRcloneRCURL == "" {
		return nil, fmt.Errorf("rclone: MEDIA_RCLONE_RC_URL is required")
	}

	return &Client{
		baseURL: strings.TrimRight(cfg.MediaRcloneRCURL, "/"),
		remote:  strings.TrimSuffix(cfg.MediaRcloneRemote, ":"),
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		user: cfg.MediaRcloneUser,
		pass: cfg.MediaRclonePass,
	}, nil
}

func (c *Client) remoteName() string {
	return c.remote + ":"
}

func (c *Client) remotePath(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, "/")
}

func fileRemotePath(guildID, category, itemPath, itemName string) string {
	p := strings.Trim(strings.ReplaceAll(itemPath, "\\", "/"), "/")
	if p == "" {
		p = itemName
	}
	if category != "" && !strings.Contains(p, "/") {
		return guildID + "/" + category + "/" + p
	}
	if !strings.HasPrefix(p, guildID+"/") {
		return guildID + "/" + p
	}
	return p
}

func categoryFromRemotePath(guildID, remotePath string) string {
	rest := strings.TrimPrefix(remotePath, guildID+"/")
	if rest == remotePath {
		return ""
	}
	parts := strings.SplitN(rest, "/", 2)
	return parts[0]
}

func localDir(dir string) string {
	dir = filepath.ToSlash(dir)
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	return dir
}

func (c *Client) Ping(ctx context.Context) error {
	var out map[string]any
	if err := c.call(ctx, "core/version", nil, &out); err != nil {
		return fmt.Errorf("%w: %v", media.ErrRemoteUnavailable, err)
	}
	return nil
}

func (c *Client) Mkdir(ctx context.Context, guildID, category string) error {
	return c.call(ctx, "operations/mkdir", map[string]any{
		"fs":     c.remoteName(),
		"remote": c.remotePath(guildID, category),
	}, nil)
}

func (c *Client) List(ctx context.Context, guildID, category string) ([]media.File, error) {
	remote := c.remotePath(guildID)
	if category != "" {
		remote = c.remotePath(guildID, category)
	}

	opt := map[string]any{}
	if category == "" {
		opt["recurse"] = true
	}

	var resp listResponse
	if err := c.call(ctx, "operations/list", map[string]any{
		"fs":     c.remoteName(),
		"remote": remote,
		"opt":    opt,
	}, &resp); err != nil {
		if isNotFound(err) {
			return nil, media.ErrNotFound
		}
		return nil, err
	}

	var files []media.File
	for _, item := range resp.List {
		if item.IsDir {
			continue
		}

		fullPath := fileRemotePath(guildID, category, item.Path, item.Name)
		cat := category
		if cat == "" {
			cat = categoryFromRemotePath(guildID, fullPath)
		}

		files = append(files, media.File{
			Path:     fullPath,
			Name:     item.Name,
			Category: cat,
		})
	}

	if len(files) == 0 {
		return nil, media.ErrNotFound
	}
	return files, nil
}

func (c *Client) Read(ctx context.Context, objectPath string) (io.ReadCloser, error) {
	tmp, err := os.CreateTemp("", "media-read-*")
	if err != nil {
		return nil, fmt.Errorf("rclone: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, err
	}

	if err := c.call(ctx, "operations/copyfile", map[string]any{
		"srcFs":     c.remoteName(),
		"srcRemote": strings.ReplaceAll(objectPath, "\\", "/"),
		"dstFs":     localDir(filepath.Dir(tmpPath)),
		"dstRemote": filepath.Base(tmpPath),
	}, nil); err != nil {
		os.Remove(tmpPath)
		if isNotFound(err) {
			return nil, media.ErrNotFound
		}
		return nil, err
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return nil, err
	}
	return &tempReadCloser{File: f, path: tmpPath}, nil
}

func (c *Client) Write(ctx context.Context, guildID, category, filename string, r io.Reader) error {
	tmp, err := os.CreateTemp("", "media-upload-*")
	if err != nil {
		return fmt.Errorf("rclone: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return fmt.Errorf("rclone: temp write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("rclone: temp close: %w", err)
	}

	return c.call(ctx, "operations/copyfile", map[string]any{
		"srcFs":     localDir(filepath.Dir(tmpPath)),
		"srcRemote": filepath.Base(tmpPath),
		"dstFs":     c.remoteName(),
		"dstRemote": c.remotePath(guildID, category, filename),
	}, nil)
}

type tempReadCloser struct {
	*os.File
	path string
}

func (t *tempReadCloser) Close() error {
	err := t.File.Close()
	os.Remove(t.path)
	return err
}

type listResponse struct {
	List []listItem `json:"list"`
}

type listItem struct {
	Path  string `json:"Path"`
	Name  string `json:"Name"`
	IsDir bool   `json:"IsDir"`
}

type rcError struct {
	Status  int    `json:"status"`
	Message string `json:"error"`
}

func (e *rcError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("rclone rc status %d", e.Status)
}

func (c *Client) call(ctx context.Context, endpoint string, params any, out any) error {
	payload := params
	if payload == nil {
		payload = map[string]any{}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/"+endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.user != "" {
		req.SetBasicAuth(c.user, c.pass)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", media.ErrRemoteUnavailable, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var rcErr rcError
	if err := json.Unmarshal(raw, &rcErr); err == nil && rcErr.Message != "" {
		return &rcErr
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("rclone rc http %d: %s", resp.StatusCode, string(raw))
	}

	if out != nil && len(raw) > 0 && raw[0] == '{' {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("rclone: decode response: %w", err)
		}
	}
	return nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, media.ErrNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "directory not found")
}
