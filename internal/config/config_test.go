package config

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// The full reference documents all settings; Docker keeps a small quick start.
const (
	composePath   = "docker/compose.yaml"
	rootEnvPath   = ".env.example"
	dockerEnvPath = "docker/.env.example"
)

// intentionallyAbsent lists variables a file may legitimately omit, with the
// reason. Empty is the healthy state; an entry is a decision, not a backlog.
var intentionallyAbsent = map[string]map[string]string{}

// envTag pulls the variable name out of an `env:"NAME,required"` struct tag.
var envTag = regexp.MustCompile(`^([A-Z0-9_]+)`)

// declaredEnvVars reads the names off Config itself rather than off the source
// text, so a renamed field cannot drift from what the test checks.
func declaredEnvVars(t *testing.T) []string {
	t.Helper()

	var names []string
	fields := reflect.TypeOf(Config{})
	for i := 0; i < fields.NumField(); i++ {
		tag, ok := fields.Field(i).Tag.Lookup("env")
		if !ok {
			continue
		}
		if m := envTag.FindStringSubmatch(tag); m != nil {
			names = append(names, m[1])
		}
	}
	if len(names) == 0 {
		t.Fatal("no env tags found on Config; this test would pass vacuously")
	}
	return names
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(raw)
}

func exempt(name, file string) bool {
	_, ok := intentionallyAbsent[name][file]
	return ok
}

// An undocumented variable may as well not exist: nobody discovers it except by
// reading the struct.
func TestEveryConfigVarIsDocumentedInTheEnvExamples(t *testing.T) {
	for _, path := range []string{rootEnvPath} {
		contents := readRepoFile(t, path)

		for _, name := range declaredEnvVars(t) {
			if exempt(name, path) {
				continue
			}
			// A commented-out entry still documents an optional variable, and
			// is the right shape for one whose default is "unset".
			if !regexp.MustCompile(`(?m)^#?\s*` + name + `=`).MatchString(contents) {
				t.Errorf("%s is not documented in %s", name, path)
			}
		}
	}
}

// The control: without it the two tests above would pass just as happily if
// declaredEnvVars quietly returned nothing useful.
func TestDeclaredEnvVarsFindsTheKnownOnes(t *testing.T) {
	names := declaredEnvVars(t)

	found := make(map[string]bool, len(names))
	for _, n := range names {
		found[n] = true
	}
	for _, want := range []string{"DISCORD_TOKEN", "STORAGE_PATH", "HEALTHCHECK_PATH"} {
		if !found[want] {
			t.Errorf("declaredEnvVars did not find %s", want)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}
		dir = parent
	}
}

// The example files are parsed with the real parser, not grepped.
//
// An earlier version of this file only checked that each variable name
// appeared somewhere, and passed happily on
// `COMMAND_TIMEOUT=30sollama|http://localhost:11434/v1|...` — a botched
// edit that spliced one setting into the middle of another's example. The name
// was present, so the check was satisfied, and the file it was guarding was
// nonsense. A value that cannot be parsed is the failure worth catching:
// durations, ints, bools and floats all go through the same tags here.
func TestEnvExamplesParseIntoAValidConfig(t *testing.T) {
	for _, path := range []string{rootEnvPath, dockerEnvPath} {
		t.Run(path, func(t *testing.T) {
			values, err := godotenv.Read(filepath.Join(repoRoot(t), filepath.FromSlash(path)))
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			// Required fields are deliberately blank in an example file; give
			// them a value so the parse exercises every other field rather
			// than stopping at the first required one.
			for _, name := range []string{"DISCORD_TOKEN", "TASKS_PATH"} {
				if values[name] == "" {
					values[name] = "placeholder"
				}
			}

			var cfg Config
			if err := env.ParseWithOptions(&cfg, env.Options{Environment: values}); err != nil {
				t.Errorf("%s does not parse: %v", path, err)
			}
		})
	}
}

// The control: the check above is only worth having if a bad value actually
// fails it.
func TestEnvExampleParsingRejectsABadValue(t *testing.T) {
	values := map[string]string{
		"DISCORD_TOKEN":      "placeholder",
		"TASKS_PATH":         "placeholder",
		"WS_SILENCE_TIMEOUT": "2m-ollama|http://localhost:11434/v1|llama3.1",
	}

	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: values}); err == nil {
		t.Error("a spliced duration parsed cleanly; this check would not have caught the real thing")
	}
}

// TestDockerExampleUsesKnownSettings keeps the small quick-start example valid.
func TestDockerExampleUsesKnownSettings(t *testing.T) {
	values, err := godotenv.Read(filepath.Join(repoRoot(t), dockerEnvPath))
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{"IMAGE_TAG": true}
	for _, name := range declaredEnvVars(t) {
		known[name] = true
	}
	for name := range values {
		if !known[name] {
			t.Errorf("unknown Docker example setting %s", name)
		}
	}
	if _, exists := values["DISCORD_TOKEN"]; !exists {
		t.Fatal("Docker quick start omits its required token")
	}
}

// composeVar matches ${NAME} and ${NAME:-default} in the compose file.
var composeVar = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)`)

// Compose-only settings must be discoverable in the Docker example.
func TestEveryComposeVarIsDocumentedInTheDockerEnvExample(t *testing.T) {
	compose := readRepoFile(t, composePath)
	compose += readRepoFile(t, "docker/compose.traefik.yaml")
	example := readRepoFile(t, dockerEnvPath)

	seen := make(map[string]bool)
	for _, m := range composeVar.FindAllStringSubmatch(compose, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true

		if !regexp.MustCompile(`(?m)^#?\s*` + name + `=`).MatchString(example) {
			t.Errorf("%s is read by %s but documented nowhere in %s", name, composePath, dockerEnvPath)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no variables found in the compose file; this test would pass vacuously")
	}
}
