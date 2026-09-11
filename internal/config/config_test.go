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

// Every place a new setting has to be repeated by hand. Adding a field to
// Config and stopping there is silent: the bot runs fine on the developer's
// machine, reads the variable from a local .env, and quietly falls back to the
// default in Docker forever, because the compose file passes an explicit
// allowlist rather than the whole environment.
//
// That is exactly how the chat persona shipped unreachable in Docker: thirteen
// CHAT_* variables reached .env.example and none reached docker-compose.yml.
const (
	composePath   = "docker/docker-compose.yml"
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

// The compose file passes an explicit allowlist, so anything missing from it is
// unreachable in Docker no matter what the operator puts in their .env.
func TestEveryConfigVarIsPassedThroughDockerCompose(t *testing.T) {
	compose := readRepoFile(t, composePath)

	for _, name := range declaredEnvVars(t) {
		if exempt(name, composePath) {
			continue
		}
		// Matches both ${NAME} and ${NAME:-default}.
		if !regexp.MustCompile(`\$\{` + name + `[:}]`).MatchString(compose) {
			t.Errorf("%s is not passed through %s, so it cannot be set in Docker", name, composePath)
		}
	}
}

// An undocumented variable may as well not exist: nobody discovers it except by
// reading the struct.
func TestEveryConfigVarIsDocumentedInTheEnvExamples(t *testing.T) {
	for _, path := range []string{rootEnvPath, dockerEnvPath} {
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
	for _, want := range []string{"DISCORD_TOKEN", "STORAGE_PATH", "CHAT_ENABLED"} {
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
// `CHAT_REQUEST_TIMEOUT=45sollama|http://localhost:11434/v1|...` — a botched
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
		"DISCORD_TOKEN":        "placeholder",
		"TASKS_PATH":           "placeholder",
		"CHAT_REQUEST_TIMEOUT": "45sollama|http://localhost:11434/v1|llama3.1",
	}

	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: values}); err == nil {
		t.Error("a spliced duration parsed cleanly; this check would not have caught the real thing")
	}
}
