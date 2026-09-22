package welcome

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Placeholders a template can use.
const (
	placeUser   = "{user}"
	placeName   = "{name}"
	placeServer = "{server}"
	placeRole   = "{role}"
)

// maxMessage is Discord's limit on a message's content.
const maxMessage = 2000

// Vars fill a template's placeholders.
type Vars struct {
	// UserID is the person being welcomed; {user} becomes a mention of them.
	UserID string
	// Name is how they are shown in the server, for {name}.
	Name   string
	Server string
	// Role is the role's name, for {role} — as text, never a role mention,
	// which would notify everyone wearing it.
	Role string
}

// Channel is one channel a template may name.
type Channel struct {
	ID   string
	Name string
}

// Render fills a template for one person.
//
// "#channel-name" becomes a link to that channel. Templates are written by
// pasting text copied out of Discord, and copied text carries a channel as
// "#introduction", not as the "<#id>" a message needs to link it — so without
// this every channel in a pasted template would arrive as plain grey text.
// A name that matches no channel is left as it was written.
func Render(template string, v Vars, channels []Channel) string {
	template = invisible.Replace(template)
	out := strings.NewReplacer(
		placeUser, "<@"+v.UserID+">",
		placeName, v.Name,
		placeServer, v.Server,
		placeRole, v.Role,
	).Replace(template)
	return linkChannels(out, channels)
}

// invisible removes characters Discord puts into copied text that nobody can
// see: a mention pasted from Discord arrives as "\u2060#name". Invisible, but
// in the way — the warning about unmatched names looks for a "#" after a
// space, and did not see one. The zero-width joiner stays: emoji are built
// with it.
var invisible = strings.NewReplacer("\u2060", "", "\u200b", "", "\ufeff", "")

// linkChannels turns "#name" into "<#id>" for channels that exist, longest
// name first so "#roles-info" is not taken for "#roles".
func linkChannels(text string, channels []Channel) string {
	if !strings.Contains(text, "#") || len(channels) == 0 {
		return text
	}
	sorted := append([]Channel(nil), channels...)
	sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i].Name) > len(sorted[j].Name) })

	var b strings.Builder
	for i := 0; i < len(text); {
		// "<#123>" is already a link.
		if text[i] != '#' || (i > 0 && text[i-1] == '<') {
			b.WriteByte(text[i])
			i++
			continue
		}
		rest := text[i+1:]
		linked := false
		for _, c := range sorted {
			if c.Name == "" || len(rest) < len(c.Name) || !strings.EqualFold(rest[:len(c.Name)], c.Name) {
				continue
			}
			if next, _ := utf8.DecodeRuneInString(rest[len(c.Name):]); isNameRune(next) {
				continue
			}
			b.WriteString("<#" + c.ID + ">")
			i += 1 + len(c.Name)
			linked = true
			break
		}
		if !linked {
			b.WriteByte('#')
			i++
		}
	}
	return b.String()
}

// isNameRune reports whether r could continue a channel name, which is what
// stops "#roles" matching the start of "#roleplay".
func isNameRune(r rune) bool {
	return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_')
}

// TooLong reports whether a rendered message is over Discord's limit.
func TooLong(message string) bool {
	return utf8.RuneCountInString(message) > maxMessage
}
