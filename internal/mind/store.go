package mind

import (
	"sync"
)

// Store holds Core and per-guild states. Safe for concurrent use.
type Store struct {
	core   *Core
	root   string
	mu     sync.RWMutex
	guilds map[string]*GuildState
}

// NewStore creates a Store with data root (e.g. "data/mind"). Calls InitDefaultCore so core files exist.
func NewStore(dataRoot string) *Store {
	if dataRoot == "" {
		dataRoot = "data/mind"
	}
	InitDefaultCore(dataRoot)
	s := &Store{
		core:   NewCore(dataRoot),
		root:   dataRoot,
		guilds: make(map[string]*GuildState),
	}
	_ = s.core.Load()
	return s
}

// Core returns the global core (biology, identity, worldview).
func (s *Store) Core() *Core {
	return s.core
}

// Guild returns GuildState for guildID, creating and loading if needed.
func (s *Store) Guild(guildID string) *GuildState {
	s.mu.RLock()
	g := s.guilds[guildID]
	s.mu.RUnlock()
	if g != nil {
		return g
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if g = s.guilds[guildID]; g != nil {
		return g
	}
	g = NewGuildState(s.root, guildID)
	_ = g.Load()
	s.guilds[guildID] = g
	return g
}

// AllGuildIDs returns all known guild IDs (for scheduler iteration).
func (s *Store) AllGuildIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.guilds))
	for id := range s.guilds {
		ids = append(ids, id)
	}
	return ids
}
