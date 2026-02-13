package mind

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Core holds global mind data (biology, identity, worldview). Thread-safe.
type Core struct {
	root   string
	mu     sync.RWMutex
	bio    *Biology
	world  *Worldview
	identityBytes []byte
}

// NewCore creates Core with data root at data/mind/.
func NewCore(dataRoot string) *Core {
	if dataRoot == "" {
		dataRoot = "data/mind"
	}
	return &Core{root: filepath.Join(dataRoot, "core")}
}

// InitDefaultCore creates core directory and default files if missing. Safe to call at startup.
func InitDefaultCore(dataRoot string) {
	if dataRoot == "" {
		dataRoot = "data/mind"
	}
	coreDir := filepath.Join(dataRoot, "core")
	_ = os.MkdirAll(coreDir, 0755)

	identityPath := filepath.Join(coreDir, CoreIdentity)
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		// Copy from legacy chat prompt so identity exists
		if b, err := os.ReadFile("data/chat.prompt.md"); err == nil {
			_ = os.WriteFile(identityPath, b, 0644)
		}
	}

	bioPath := filepath.Join(coreDir, CoreBiology)
	if _, err := os.Stat(bioPath); os.IsNotExist(err) {
		def := &Biology{
			Temperament: "dominant_reserved",
			Age:         28,
			SpeechStyle:  "sharp_aristocratic",
			Dominance:    0.8,
			EmoReact:     0.6,
		}
		b, _ := json.MarshalIndent(def, "", "  ")
		_ = os.WriteFile(bioPath, b, 0644)
	}

	worldPath := filepath.Join(coreDir, CoreWorldview)
	if _, err := os.Stat(worldPath); os.IsNotExist(err) {
		def := &Worldview{TrustInPeople: 0.5, Cynicism: 0.4, Openness: 0.6, LoyaltyBias: 0.5}
		b, _ := json.MarshalIndent(def, "", "  ")
		_ = os.WriteFile(worldPath, b, 0644)
	}
}

// Load reads all core files from disk. Missing files leave nil; defaults applied in Get*.
func (c *Core) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// biology
	if b, err := os.ReadFile(filepath.Join(c.root, CoreBiology)); err == nil {
		var bio Biology
		if json.Unmarshal(b, &bio) == nil {
			c.bio = &bio
		}
	}

	// worldview
	if b, err := os.ReadFile(filepath.Join(c.root, CoreWorldview)); err == nil {
		var w Worldview
		if json.Unmarshal(b, &w) == nil {
			c.world = &w
		}
	}

	// identity.md — read-only, never written by code that could be driven by LLM
	if b, err := os.ReadFile(filepath.Join(c.root, CoreIdentity)); err == nil {
		c.identityBytes = b
	}

	return nil
}

// SaveBiology writes biology.json. Only called from init/setup, not from LLM path.
func (c *Core) SaveBiology(bio *Biology) error {
	if bio == nil {
		return nil
	}
	c.mu.Lock()
	c.bio = bio
	c.mu.Unlock()
	return c.writeJSON(filepath.Join(c.root, CoreBiology), bio)
}

// SaveWorldview writes worldview.json. Deltas validated by caller (e.g. max 0.05).
func (c *Core) SaveWorldview(w *Worldview) error {
	if w == nil {
		return nil
	}
	c.mu.Lock()
	c.world = w
	c.mu.Unlock()
	return c.writeJSON(filepath.Join(c.root, CoreWorldview), w)
}

func (c *Core) writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// GetBiology returns current biology; if not loaded, returns default.
func (c *Core) GetBiology() *Biology {
	c.mu.RLock()
	b := c.bio
	c.mu.RUnlock()
	if b != nil {
		return b
	}
	return &Biology{
		Temperament: "dominant_reserved",
		Age:         28,
		SpeechStyle: "sharp_aristocratic",
		Dominance:   0.8,
		EmoReact:    0.6,
	}
}

// GetWorldview returns current worldview; if not loaded, returns default.
func (c *Core) GetWorldview() *Worldview {
	c.mu.RLock()
	w := c.world
	c.mu.RUnlock()
	if w != nil {
		return w
	}
	return &Worldview{
		TrustInPeople: 0.5,
		Cynicism:      0.4,
		Openness:      0.6,
		LoyaltyBias:   0.5,
	}
}

// GetIdentityMD returns raw identity.md content. Never nil (empty slice if missing).
func (c *Core) GetIdentityMD() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.identityBytes) > 0 {
		return c.identityBytes
	}
	return []byte{}
}
