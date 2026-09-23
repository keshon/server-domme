package mind

import (
	"strings"
	"testing"
)

// A quote cut inside a code fence does not leave the fence open in her
// prompt; one cut after the fence closed keeps its formatting.
func TestClipDoesNotLeaveAFenceOpen(t *testing.T) {
	msg := "the instruction: ```\n## guide\n`/welcome setup role:@Sub intro_channel:#intros` and a lot more text after it\n```"
	if got := clip(msg, 60); strings.Contains(got, "`") {
		t.Errorf("clip left a backtick in %q", got)
	}
	if got := clip("run `/welcome roles` to see them all, then carry on with the rest of it", 40); !strings.Contains(got, "`/welcome roles`") {
		t.Errorf("clip dropped closed code: %q", got)
	}
}
