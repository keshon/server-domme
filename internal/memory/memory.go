// Package memory keeps what the persona remembers as Markdown files: who she
// is lately, a dossier per person, one file per day of moments, the things
// she means to do, and how each conversation under way has gone so far.
//
// Files rather than the datastore because everything here is prose that a
// language model reads and writes, and that a person debugging her should be
// able to open and correct in a text editor. The scale is a handful of people
// and a few dozen moments a day, so reading a file per reply costs less than
// the model call it feeds. See docs/persona.md.
//
// Nothing here knows about Discord or about models.
package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Layout inside one guild's directory. These names are what is already on
// disk once a guild has been remembered; renaming one orphans it.
const (
	selfFile    = "self.md"
	threadsFile = "threads.md"
	peopleDir   = "people"
	daysDir     = "days"
	// arcsDir holds one file per channel with a conversation under way;
	// see Arc.
	arcsDir = "rooms"
	// forgottenDir is where /chat forget moves a guild, rather than deleting
	// it: a typed confirmation is still one mistake away from losing
	// everything she was.
	forgottenDir = ".forgotten"
)

// dayLayout names a day file and dates everything in the community's
// timezone, which is the one the people she talks to live in.
const (
	dayLayout    = "2006-01-02"
	minuteLayout = "2006-01-02 15:04"
	clockLayout  = "15:04"
)

// Store is the memory of every guild, rooted in one directory.
//
// One mutex for the whole store. Writes are rare — a few per reply — and a
// per-file lock would buy nothing but the chance to take two of them in the
// wrong order.
type Store struct {
	root string
	loc  *time.Location

	mu sync.Mutex
}

// Open returns a store rooted at dir, creating it if needed. loc is the
// community's timezone; nil means UTC.
func Open(dir string, loc *time.Location) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("memory: no directory given")
	}
	if loc == nil {
		loc = time.UTC
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("memory: create %s: %w", dir, err)
	}
	return &Store{root: dir, loc: loc}, nil
}

// Location is the timezone the store dates things in.
func (s *Store) Location() *time.Location { return s.loc }

// Root is the directory the store lives in.
func (s *Store) Root() string { return s.root }

// guildDir is where one guild's memory lives. A guild id is a Discord
// snowflake; anything else is refused rather than allowed to name a path.
func (s *Store) guildDir(guildID string) (string, error) {
	if !safeName(guildID) {
		return "", fmt.Errorf("memory: bad guild id %q", guildID)
	}
	return filepath.Join(s.root, guildID), nil
}

// safeName reports whether id can be used as a single path element.
func safeName(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		ok := r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

// Guilds lists every guild with a memory.
func (s *Store) Guilds() []string {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && safeName(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// Forget moves a guild's memory aside, returning where it went. Nothing is
// deleted; see forgottenDir.
func (s *Store) Forget(guildID string, now time.Time) (string, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	aside := filepath.Join(s.root, forgottenDir, guildID+"-"+now.UTC().Format("20060102-150405"))
	if err := os.MkdirAll(filepath.Dir(aside), 0o755); err != nil {
		return "", fmt.Errorf("memory: prepare forget: %w", err)
	}
	if err := os.Rename(dir, aside); err != nil {
		return "", fmt.Errorf("memory: forget %s: %w", guildID, err)
	}
	return aside, nil
}

// readFile returns a file's contents, or "" when it does not exist yet —
// which is the ordinary state of everything she has not learned.
func readFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("memory: read %s: %w", filepath.Base(path), err)
	}
	return strings.ReplaceAll(string(raw), "\r\n", "\n"), nil
}

// writeFile replaces a file by writing beside it and renaming over it, so a
// crash leaves the old version or the new one, never half of each.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("memory: create %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("memory: write %s: %w", filepath.Base(path), err)
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("memory: write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("memory: write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("memory: replace %s: %w", filepath.Base(path), err)
	}
	return nil
}

// oneLine flattens text that is stored as a single Markdown line. A newline
// in a moment or a note would start a new item that nothing wrote.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// errBadID refuses an id that could name a path outside the store.
func errBadID(id string) error {
	return fmt.Errorf("memory: bad id %q", id)
}
