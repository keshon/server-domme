package mind

import (
	"regexp"
	"strings"
)

// InnerVoiceNote asks for a private thought ahead of the message.
//
// Cognitum's liveliest behaviour came from exactly this — a separate call that
// produced one line of what the character privately made of the moment, fed
// to the reply as "currently thinking". Here it is one call rather than two:
// the free relays are the bottleneck, and a second request per reply would
// halve how many replies they can carry.
//
// A tag rather than a "THOUGHT:" label, because ai.Clean strips a leading
// label and cuts the reply at any later line shaped like one — the same
// defence that stops her writing other people's lines would otherwise eat
// the message itself.
//
// The first wording asked what she "makes of this", and the relays answered
// with a plan — "they want a quick rename snippet; i'll give a minimal
// example" — which is an assistant deciding how to help, and the reply
// followed it there. Asking for what she feels about it and about them is
// what the thought is for.
const InnerVoiceNote = "Before your message, write one private line between <inner> and </inner>: " +
	"how this lands with you and what you think of them for it — a reaction, not a plan for your answer. " +
	"Nobody will see it. Then, on a new line, write your message, coloured by that reaction " +
	"but not a restatement of it."

// innerBlock accepts the closing tags relays actually wrote as well as the
// right one. "<inner>...<inner>" followed by the message was a quarter of the
// failures when this was first measured; the thought's end is still
// unambiguous there, so discarding the reply would be waste.
var (
	innerBlock   = regexp.MustCompile(`(?is)<inner>(.*?)(?:</\s*inner>|<\\inner>|<inner>)`)
	innerUnended = regexp.MustCompile(`(?is)</?\s*\\?inner>`)
)

// SplitThought separates her private thought from the message.
//
// ok is false when the reply cannot be posted safely: an <inner> that never
// closed, where there is no telling where the thought ends, or nothing left
// once the thought is taken out. Either way the reply is discarded rather than
// posted, because a private thought reaching the channel is worse than a
// failed generation — that is retried, and this cannot be taken back.
//
// A reply with no tag at all is posted as it is: the model ignored the
// instruction, which costs the thought but not the message.
func SplitThought(reply string) (thought, message string, ok bool) {
	var thoughts []string
	message = innerBlock.ReplaceAllStringFunc(reply, func(block string) string {
		if m := innerBlock.FindStringSubmatch(block); len(m) == 2 {
			if t := strings.TrimSpace(m[1]); t != "" {
				thoughts = append(thoughts, t)
			}
		}
		return ""
	})
	if innerUnended.MatchString(message) {
		return "", "", false
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "", "", false
	}
	return strings.Join(thoughts, " "), message, true
}
