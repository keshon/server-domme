package welcome

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

// runSetup sets where a role's intro and welcome are posted, or shows it.
func runSetup(ctx *adapter.SlashInteractionContext, opts options) error {
	roleID := roleIDOf(opts)

	intro, hasIntro := opts[optIntro]
	welcome, hasWelcome := opts[optWelcome]
	_, hasIntroNotify := opts[optIntroNotify]
	_, hasWelcomeNotify := opts[optWelcomeNotify]
	if !hasIntro && !hasWelcome && !hasIntroNotify && !hasWelcomeNotify {
		return respond(ctx, describeRole(ctx, roleID))
	}

	err := ctx.Storage.UpdateWelcomeRole(ctx.GuildID(), roleID, func(w *storage.WelcomeRole) {
		if hasIntro {
			w.IntroChannel = intro.StringValue()
		}
		if hasWelcome {
			w.WelcomeChannel = welcome.StringValue()
		}
		if hasIntroNotify {
			w.IntroNotifyAll = optBool(opts, optIntroNotify)
		}
		if hasWelcomeNotify {
			w.WelcomeNotifyAll = optBool(opts, optWelcomeNotify)
		}
	})
	if err != nil {
		return fmt.Errorf("welcome: setup: %w", err)
	}

	msg := describeRole(ctx, roleID)
	// Warned now rather than discovered when welcoming someone.
	for _, ch := range []string{optString(opts, optIntro), optString(opts, optWelcome)} {
		if ch == "" {
			continue
		}
		if why := ctx.API.CanPostIn(ch, ctx.GuildID()); why != "" {
			msg += "\n\n⚠️ " + why
		}
	}
	return respond(ctx, msg)
}

// describeRole is a role's welcome settings in a few lines.
func describeRole(ctx *adapter.SlashInteractionContext, roleID string) string {
	w := ctx.Storage.WelcomeRoleFor(ctx.GuildID(), roleID)
	if w == nil {
		return fmt.Sprintf("<@&%s> has no welcome set up. `/welcome setup` picks the channels, `/welcome template` writes the texts.", roleID)
	}
	return fmt.Sprintf("<@&%s>\n%s\n%s\n%s", roleID, partLines(w), notifyLine(w), gifLine(w, len(ctx.Storage.WelcomeGifs(ctx.GuildID(), ""))))
}

// notifyLine is whether intro and welcome texts may ping beyond the newcomer.
func notifyLine(w *storage.WelcomeRole) string {
	switch {
	case w.IntroNotifyAll && w.WelcomeNotifyAll:
		return "🔔 **Pings** · intro and welcome may ping @everyone, @here and roles"
	case w.IntroNotifyAll:
		return "🔔 **Pings** · intro may ping @everyone, @here and roles; welcome only pings the newcomer"
	case w.WelcomeNotifyAll:
		return "🔔 **Pings** · welcome may ping @everyone, @here and roles; intro only pings the newcomer"
	default:
		return "🔕 **Pings** · only the person being welcomed"
	}
}

// gifLine is where a role's welcome gifs come from; shared is how many the
// shared pool has.
func gifLine(w *storage.WelcomeRole, shared int) string {
	switch {
	case len(w.Gifs) > 0:
		return fmt.Sprintf("🎞️ **Gifs** · %d of its own", len(w.Gifs))
	case shared > 0:
		return fmt.Sprintf("🎞️ **Gifs** · the shared pool's %d", shared)
	default:
		return "➖ **Gifs** none — welcomes go out without one"
	}
}

// partLines are a role's intro and welcome, one line each, marked by whether
// they are ready to post.
func partLines(w *storage.WelcomeRole) string {
	return partLine("Intro", w.IntroChannel, w.IntroTemplate) + "\n" +
		partLine("Welcome", w.WelcomeChannel, w.WelcomeTemplate)
}

func partLine(label, channel, template string) string {
	hasText := strings.TrimSpace(template) != ""
	chars := fmt.Sprintf("%d characters", len([]rune(template)))
	switch {
	case channel != "" && hasText:
		return fmt.Sprintf("✅ **%s** → <#%s> · %s", label, channel, chars)
	case hasText:
		return fmt.Sprintf("⚠️ **%s** · %s, but no channel — `/welcome setup`", label, chars)
	case channel != "":
		return fmt.Sprintf("⚠️ **%s** → <#%s>, but no text yet — `/welcome template`", label, channel)
	default:
		return fmt.Sprintf("➖ **%s** not set up", label)
	}
}

// runTemplate opens the editor for a role's intro or welcome text.
//
// A modal rather than a command option: an option is one line, and the texts
// this is for are paragraphs with blank lines between them.
func runTemplate(ctx *adapter.SlashInteractionContext, opts options) error {
	roleID := roleIDOf(opts)
	kind := optString(opts, optKind)

	current := ""
	if w := ctx.Storage.WelcomeRoleFor(ctx.GuildID(), roleID); w != nil {
		current = w.IntroTemplate
		if kind == kindWelcome {
			current = w.WelcomeTemplate
		}
	}

	title := "Intro text"
	if kind == kindWelcome {
		title = "Welcome text"
	}
	title += " for " + roleName(ctx, roleID)
	if r := []rune(title); len(r) > 45 {
		title = string(r[:45])
	}

	return ctx.Responder.OpenModal(adapter.Modal{
		CustomID: modalPrefix + kind + ":" + roleID,
		Title:    title,
		Fields: []adapter.ModalField{{
			CustomID:    modalField,
			Label:       "Text ({user} {name} {server} {role})",
			Placeholder: "Paste it as written. #channel-names become links. Empty removes it.",
			Value:       current,
			Required:    false,
			MaxLength:   maxMessage,
		}},
	})
}

// ModalSubmit saves a template written in the editor.
func (c *WelcomeCommand) ModalSubmit(ctx *adapter.ModalSubmitContext) error {
	if !isAdmin(ctx) {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Only administrators can change welcome texts.",
			Color:       reply.EmbedColor,
		})
	}

	rest := strings.TrimPrefix(ctx.ComponentID, modalPrefix)
	kind, roleID, ok := strings.Cut(rest, ":")
	if !ok || (kind != kindIntro && kind != kindWelcome) || roleID == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "That editor is out of date. Open it again with `/welcome template`.",
			Color:       reply.EmbedColor,
		})
	}

	// Acknowledged before anything else, and answered by editing that in:
	// looking up archived threads is a request per channel, and Discord
	// gives a modal three seconds before it tells the administrator
	// "Something went wrong" — which it did, on a server with a few dozen
	// channels, while the text was being saved behind it.
	if err := ctx.DeferEphemeral(); err != nil {
		return fmt.Errorf("welcome: acknowledge template: %w", err)
	}
	answer := func(msg string) error {
		return ctx.FollowupEphemeral(&adapter.Embed{Description: msg, Color: reply.EmbedColor})
	}

	text := strings.TrimSpace(ctx.ModalValue(modalField))
	known, _ := ctx.API.GuildChannels(ctx.GuildID())
	text = linkArchivedModal(ctx, text, known)
	err := ctx.Storage.UpdateWelcomeRole(ctx.GuildID(), roleID, func(w *storage.WelcomeRole) {
		if kind == kindIntro {
			w.IntroTemplate = text
		} else {
			w.WelcomeTemplate = text
		}
	})
	if err != nil {
		_ = answer("The text could not be saved. Try again.")
		return fmt.Errorf("welcome: save template: %w", err)
	}
	if text == "" {
		return answer(fmt.Sprintf("Removed the %s text for <@&%s>.", kind, roleID))
	}

	// Rendered for the person saving it, so they see the links and their
	// own name where a newcomer's will go.
	name := ctx.Invoker.DisplayName
	if name == "" {
		name = "there"
	}
	server := "the server"
	if info, err := ctx.API.GuildInfo(ctx.GuildID()); err == nil {
		server = info.Name
	}
	role := roleID
	if rName, err := ctx.API.RoleName(ctx.GuildID(), roleID); err == nil {
		role = rName
	}
	v := Vars{UserID: ctx.UserID(), Name: name, Server: server, Role: role}
	rendered := Render(text, v, known)

	msg := fmt.Sprintf("Saved the %s text for <@&%s>. With you in it, it reads:\n\n%s", kind, roleID, rendered)
	if missing := unlinked(rendered); len(missing) > 0 {
		msg += "\n\n⚠️ No channel or thread matches " + strings.Join(missing, ", ") + unlinkedHint
	}
	if TooLong(rendered) {
		msg += "\n\n⚠️ It is over Discord's 2000 characters with a name in it, and will be refused until shortened."
	}
	return answer(trimEmbed(msg))
}

// runPreview shows a role's texts as they would be posted, and where.
func runPreview(ctx *adapter.SlashInteractionContext, opts options) error {
	roleID := roleIDOf(opts)
	w := ctx.Storage.WelcomeRoleFor(ctx.GuildID(), roleID)
	if w == nil {
		return respond(ctx, describeRole(ctx, roleID))
	}

	memberID := ctx.UserID()
	memberName := ctx.Invoker.DisplayName
	if memberName == "" {
		memberName = "there"
	}
	if uid := optString(opts, optUser); uid != "" {
		if m, err := ctx.API.Member(ctx.GuildID(), uid); err == nil {
			memberID = m.UserID
			memberName = m.DisplayName()
		}
	}
	server := "the server"
	if info, err := ctx.API.GuildInfo(ctx.GuildID()); err == nil {
		server = info.Name
	}
	v := Vars{UserID: memberID, Name: memberName, Server: server, Role: roleName(ctx, roleID)}
	channels, _ := ctx.API.GuildChannels(ctx.GuildID())

	var b strings.Builder
	fmt.Fprintf(&b, "Preview for <@%s> as <@&%s> — nothing is posted.\n", memberID, roleID)
	for _, p := range []struct{ label, channel, template string }{
		{"Intro", w.IntroChannel, w.IntroTemplate},
		{"Welcome", w.WelcomeChannel, w.WelcomeTemplate},
	} {
		where := "no channel set"
		if p.channel != "" {
			where = "<#" + p.channel + ">"
		}
		fmt.Fprintf(&b, "\n**%s** → %s\n", p.label, where)
		if strings.TrimSpace(p.template) == "" {
			b.WriteString("no text yet\n")
			continue
		}
		rendered := Render(p.template, v, channels)
		b.WriteString(rendered + "\n")
		if missing := unlinked(rendered); len(missing) > 0 {
			b.WriteString("⚠️ No channel or thread matches " + strings.Join(missing, ", ") + unlinkedHint + "\n")
		}
	}
	if gifs, shared := ctx.Storage.WelcomeGifPool(ctx.GuildID(), roleID); len(gifs) > 0 {
		from := "its own"
		if shared {
			from = "the shared pool's"
		}
		fmt.Fprintf(&b, "\nThe welcome also gets one of %s %d gifs at random.", from, len(gifs))
	}
	return respond(ctx, trimEmbed(b.String()))
}

// runRoles lists every role with a welcome, a field each.
func runRoles(ctx *adapter.SlashInteractionContext) error {
	roles := ctx.Storage.WelcomeRoles(ctx.GuildID())
	if len(roles) == 0 {
		return respond(ctx, "No role has a welcome set up yet. `/welcome setup` is where to start.")
	}
	names := make(map[string]string, len(roles))
	for _, r := range roles {
		names[r.RoleID] = roleLabel(ctx, r.RoleID)
	}
	slices.SortFunc(roles, func(a, b storage.WelcomeRole) int {
		return strings.Compare(strings.ToLower(names[a.RoleID]), strings.ToLower(names[b.RoleID]))
	})

	shared := len(ctx.Storage.WelcomeGifs(ctx.GuildID(), ""))
	embed := &adapter.Embed{Title: "👋 Welcomes", Color: reply.EmbedColor}
	for i, r := range roles {
		// Discord takes 25 fields; the rest are named, not described.
		if i == maxFields-1 && len(roles) > maxFields {
			embed.Fields = append(embed.Fields, adapter.EmbedField{
				Name:  fmt.Sprintf("…and %d more", len(roles)-i),
				Value: "`/welcome setup role:` shows any one of them.",
			})
			break
		}
		rr := r
		embed.Fields = append(embed.Fields, adapter.EmbedField{Name: names[r.RoleID], Value: partLines(&rr) + "\n" + gifLine(&rr, shared)})
	}

	embed.Footer = "👋 Welcomes preview shows a role's texts · /welcome gifs lists the gifs"
	return ctx.RespondEphemeral(embed)
}

// maxFields is how many fields Discord allows in one embed.
const maxFields = 25

// roleLabel is a role's name as plain text, for where Discord does not turn
// a mention into one, such as an embed field's name.
func roleLabel(ctx *adapter.SlashInteractionContext, roleID string) string {
	if name, err := ctx.API.RoleName(ctx.GuildID(), roleID); err == nil {
		return "@" + name
	}
	return "Deleted role " + roleID
}

// runMove gives a role's welcome to another role, so one written and tried
// on a test role and test channels is not written again for the real ones.
func runMove(ctx *adapter.SlashInteractionContext, opts options) error {
	from := optString(opts, optFrom)
	to := optString(opts, optTo)
	keep := optBool(opts, optKeep)
	intro, hasIntro := opts[optIntro]
	welcome, hasWelcome := opts[optWelcome]

	switch {
	case from == to:
		return respond(ctx, "Those are the same role.")
	case to == ctx.GuildID():
		return respond(ctx, "Everyone has @everyone, so it cannot say who is being welcomed. Pick a role.")
	}

	err := ctx.Storage.MoveWelcomeRole(ctx.GuildID(), from, to, keep, func(w *storage.WelcomeRole) {
		if hasIntro {
			w.IntroChannel = intro.StringValue()
		}
		if hasWelcome {
			w.WelcomeChannel = welcome.StringValue()
		}
	})
	switch {
	case errors.Is(err, storage.ErrWelcomeRoleMissing):
		return respond(ctx, fmt.Sprintf("<@&%s> has no welcome to move. `/welcome roles` lists the ones that do.", from))
	case errors.Is(err, storage.ErrWelcomeRoleTaken):
		return respond(ctx, fmt.Sprintf("<@&%s> already has a welcome, and it is not written over. "+
			"`/welcome remove confirm:yes` it first if this one should take its place.", to))
	case err != nil:
		return fmt.Errorf("welcome: move: %w", err)
	}

	verb := "Moved"
	if keep {
		verb = "Copied"
	}
	msg := fmt.Sprintf("%s the welcome from <@&%s> to <@&%s>.\n\n%s", verb, from, to, describeRole(ctx, to))
	w := ctx.Storage.WelcomeRoleFor(ctx.GuildID(), to)
	for _, ch := range []string{w.IntroChannel, w.WelcomeChannel} {
		if ch == "" {
			continue
		}
		if why := ctx.API.CanPostIn(ch, ctx.GuildID()); why != "" {
			msg += "\n\n⚠️ " + why
		}
	}
	if !hasIntro || !hasWelcome {
		msg += "\n\nChannels not given were kept as they were — `/welcome setup` changes them."
	}
	msg += "\n`/welcome preview` shows the texts as they would be posted."
	return respond(ctx, msg)
}

func runRemove(ctx *adapter.SlashInteractionContext, opts options) error {
	roleID := roleIDOf(opts)
	if strings.ToLower(optString(opts, "confirm")) != "yes" {
		return respond(ctx, "Action not confirmed. Please type 'yes' to proceed.")
	}
	if ctx.Storage.WelcomeRoleFor(ctx.GuildID(), roleID) == nil {
		return respond(ctx, fmt.Sprintf("<@&%s> had no welcome set up.", roleID))
	}
	if err := ctx.Storage.RemoveWelcomeRole(ctx.GuildID(), roleID); err != nil {
		return fmt.Errorf("welcome: remove: %w", err)
	}
	return respond(ctx, fmt.Sprintf("Removed the welcome for <@&%s>. Who was already welcomed is still remembered.", roleID))
}

// runGifs manages the gif pools: a role's own with `role:`, the shared one
// without. A role with no gifs of its own falls back to the shared pool, so
// a server that wants one set for everyone never has to name a role.
func runGifs(ctx *adapter.SlashInteractionContext, sub string, opts options) error {
	roleID := roleIDOf(opts)
	pool := "the shared pool"
	if roleID != "" {
		pool = fmt.Sprintf("<@&%s>'s gifs", roleID)
	}

	switch sub {
	case subGifAdd:
		link := strings.TrimSpace(optString(opts, optURL))
		if u, err := url.Parse(link); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return respond(ctx, "That is not a link. Paste the gif's address, starting with https://.")
		}
		hadOwn := len(ctx.Storage.WelcomeGifs(ctx.GuildID(), roleID)) > 0
		if err := ctx.Storage.AddWelcomeGif(ctx.GuildID(), roleID, link); err != nil {
			if errors.Is(err, storage.ErrWelcomeGifsFull) {
				return respond(ctx, "Not added: "+err.Error()+".")
			}
			return fmt.Errorf("welcome: add gif: %w", err)
		}
		// The page is read for the gif file behind it, which can take
		// longer than Discord waits for an answer.
		if err := ctx.DeferEphemeral(); err != nil {
			return fmt.Errorf("welcome: acknowledge gif: %w", err)
		}
		msg := fmt.Sprintf("Added to %s — %d now.", pool, len(ctx.Storage.WelcomeGifs(ctx.GuildID(), roleID)))
		if roleID != "" && !hadOwn {
			msg += fmt.Sprintf("\nIts first own gif: welcomes for <@&%s> no longer pick from the shared pool.", roleID)
		}
		if media := gifMedia(ctx.Storage, ctx.GuildID(), link); media != "" {
			msg += "\nIt will be posted as the gif itself, without the link."
		} else {
			msg += "\n⚠️ I could not find the gif file behind that link, so it will be posted as the link. " +
				"A link straight to the file (ending in .gif) always works."
		}
		return ctx.FollowupEphemeral(&adapter.Embed{Description: msg + "\n" + link, Color: reply.EmbedColor})

	case subGifRemove:
		removed, err := ctx.Storage.RemoveWelcomeGif(ctx.GuildID(), roleID, optString(opts, optURL))
		if err != nil {
			return fmt.Errorf("welcome: remove gif: %w", err)
		}
		if !removed {
			return respond(ctx, fmt.Sprintf("That link is not in %s. `/welcome gifs` shows every pool.", pool))
		}
		msg := "Removed from " + pool + "."
		if roleID != "" && len(ctx.Storage.WelcomeGifs(ctx.GuildID(), roleID)) == 0 {
			msg += fmt.Sprintf("\nIt has none of its own left, so welcomes for <@&%s> pick from the shared pool again.", roleID)
		}
		return respond(ctx, msg)

	default:
		if roleID != "" {
			return respond(ctx, trimEmbed(describeGifs(ctx, roleID)))
		}
		return ctx.RespondEphemeral(gifsEmbed(ctx))
	}
}

// describeGifs is what one role's welcomes pick from.
func describeGifs(ctx *adapter.SlashInteractionContext, roleID string) string {
	gifs, shared := ctx.Storage.WelcomeGifPool(ctx.GuildID(), roleID)
	switch {
	case len(gifs) == 0:
		return fmt.Sprintf("<@&%s> has no gifs, and the shared pool is empty — its welcomes go out without one. "+
			"`/welcome gif-add role:` adds one.", roleID)
	case shared:
		return fmt.Sprintf("<@&%s> has no gifs of its own, so its welcomes pick from the shared pool:\n%s",
			roleID, strings.Join(gifs, "\n"))
	default:
		return fmt.Sprintf("Welcomes for <@&%s> pick one of these at random:\n%s", roleID, strings.Join(gifs, "\n"))
	}
}

// gifsEmbed lists every pool: the shared one, then each role with its own.
func gifsEmbed(ctx *adapter.SlashInteractionContext) *adapter.Embed {
	embed := &adapter.Embed{Title: "🎞️ Welcome gifs", Color: reply.EmbedColor}
	shared := ctx.Storage.WelcomeGifs(ctx.GuildID(), "")
	sharedValue := "Empty."
	if len(shared) > 0 {
		sharedValue = linkList(shared)
	}
	embed.Fields = append(embed.Fields, adapter.EmbedField{
		Name: fmt.Sprintf("Shared pool · %d", len(shared)), Value: sharedValue,
	})

	var own []storage.WelcomeRole
	for _, r := range ctx.Storage.WelcomeRoles(ctx.GuildID()) {
		if len(r.Gifs) > 0 {
			own = append(own, r)
		}
	}
	slices.SortFunc(own, func(a, b storage.WelcomeRole) int {
		return strings.Compare(strings.ToLower(roleLabel(ctx, a.RoleID)), strings.ToLower(roleLabel(ctx, b.RoleID)))
	})
	for i, r := range own {
		if len(embed.Fields) == maxFields-1 && len(own)-i > 1 {
			embed.Fields = append(embed.Fields, adapter.EmbedField{
				Name:  fmt.Sprintf("…and %d more roles", len(own)-i),
				Value: "`/welcome gifs role:` shows any one of them.",
			})
			break
		}
		embed.Fields = append(embed.Fields, adapter.EmbedField{
			Name: fmt.Sprintf("%s · %d", roleLabel(ctx, r.RoleID), len(r.Gifs)), Value: linkList(r.Gifs),
		})
	}

	embed.Footer = "A role with no gifs of its own picks from the shared pool · add with /welcome gif-add role:"
	return embed
}

// linkList is links a line each, inside a field's 1024 characters.
func linkList(links []string) string {
	var b strings.Builder
	for i, l := range links {
		more := fmt.Sprintf("…and %d more", len(links)-i)
		if b.Len()+len(l)+1 > 1024-len(more)-1 {
			b.WriteString(more)
			break
		}
		b.WriteString(l + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// trimEmbed keeps a reply inside an embed's 4096 characters.
func trimEmbed(s string) string {
	if r := []rune(s); len(r) > 3900 {
		return string(r[:3900]) + "…"
	}
	return s
}
