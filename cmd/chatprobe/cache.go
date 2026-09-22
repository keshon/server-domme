package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/keshon/server-domme/internal/ai"
)

// cacheFile records every answer a backend gives, keyed by the exact
// messages asked, and gives the recorded answer back when the same messages
// are asked again. The relays are not deterministic; with the cache, a run
// can be repeated exactly — same answers, and with -seed the same code-side
// randomness — so a bug seen once can be seen again. See docs/persona-v3.md,
// Known failure points.
type cacheFile struct {
	path    string
	mu      sync.Mutex
	entries map[string]cached
}

type cached struct {
	Reply   string `json:"reply"`
	Backend string `json:"backend"`
}

func openCache(path string) (*cacheFile, error) {
	c := &cacheFile{path: path, entries: map[string]cached{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chatprobe: read cache: %w", err)
	}
	if err := json.Unmarshal(raw, &c.entries); err != nil {
		return nil, fmt.Errorf("chatprobe: read cache: %w", err)
	}
	return c, nil
}

// wrap returns a provider answering from the cache first. voice keeps the
// voice's answers apart from thinking's, since the pool routes them to
// different backends.
func (c *cacheFile) wrap(p ai.Provider, voice bool) *replayProvider {
	return &replayProvider{file: c, provider: p, voice: voice}
}

func (c *cacheFile) save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, err := json.MarshalIndent(c.entries, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(c.path, raw, 0o644); err != nil {
		return fmt.Errorf("chatprobe: write cache: %w", err)
	}
	return nil
}

// replayProvider is one provider — thinking or voice — through a cache.
type replayProvider struct {
	file     *cacheFile
	provider ai.Provider
	voice    bool
}

func (r *replayProvider) key(msgs []ai.Message) string {
	h := sha256.New()
	if r.voice {
		h.Write([]byte("voice\x00"))
	}
	for _, m := range msgs {
		h.Write([]byte(m.Role + "\x00" + m.Content + "\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Generate implements ai.Provider.
func (r *replayProvider) Generate(ctx context.Context, msgs []ai.Message) (string, error) {
	reply, _, err := r.GenerateNamed(ctx, msgs)
	return reply, err
}

// GenerateNamed answers from the cache, or asks and records.
func (r *replayProvider) GenerateNamed(ctx context.Context, msgs []ai.Message) (string, string, error) {
	k := r.key(msgs)
	r.file.mu.Lock()
	hit, ok := r.file.entries[k]
	r.file.mu.Unlock()
	if ok {
		return hit.Reply, hit.Backend + " (cached)", nil
	}
	type named interface {
		GenerateNamed(context.Context, []ai.Message) (string, string, error)
	}
	var reply, backend string
	var err error
	if n, isNamed := r.provider.(named); isNamed {
		reply, backend, err = n.GenerateNamed(ctx, msgs)
	} else {
		reply, err = r.provider.Generate(ctx, msgs)
	}
	if err != nil {
		return reply, backend, err
	}
	r.file.mu.Lock()
	r.file.entries[k] = cached{Reply: reply, Backend: backend}
	r.file.mu.Unlock()
	return reply, backend, nil
}
