package mind

import (
	"strings"

	"github.com/keshon/server-domme/internal/memory"
)

// saidArrow joins what someone said to what she answered, in the moment
// Said writes for an answer.
const saidArrow = " → I said: "

// observedPart is what a moment shows of what happened around her, without
// her own words: all of a moment that is not a line of hers, the other
// person's half of an answer, and nothing of a line she spoke up with on her
// own. It reports false when nothing is left.
//
// This is what her life and her impulses may rest on. Her own lines are
// what she said, not what happened: in production she once claimed to be
// building a project she had never heard of outside her voice examples,
// and a moment quoting that claim would otherwise be read back the next
// night as something she observed, and the invention would become her life.
// What she said about herself has its own path, with its own checks — see
// ReflectSelf. See docs/persona-v3.md, I.
func observedPart(mo memory.Moment) (string, bool) {
	if mo.Said == "" {
		return mo.Text, true
	}
	if i := strings.Index(mo.Text, saidArrow); i > 0 {
		return mo.Text[:i] + " (she answered)", true
	}
	return "", false
}
