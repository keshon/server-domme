package welcome

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

type options = map[string]*discordgo.ApplicationCommandInteractionDataOption

func roleIDOf(opts options) string {
	if o := opts[optRole]; o != nil {
		id, _ := o.Value.(string)
		return id
	}
	return ""
}

// runSetup sets where a role's intro and welcome are posted, or shows it.
func runSetup(context *cmdadapter.SlashInteractionContext, opts options) error {
	s, e, store := context.Session, context.Event, context.Storage
	roleID := roleIDOf(opts)

	intro, welcome := opts[optIntro], opts[optWelcome]
	if intro == nil && welcome == nil {
		return respond(s, e, describeRole(s, store, e.GuildID, roleID))
	}

	err := store.UpdateWelcomeRole(e.GuildID, roleID, func(w *storage.WelcomeRole) {
		if intro != nil {
			w.IntroChannel = intro.ChannelValue(s).ID
		}
		if welcome != nil {
			w.WelcomeChannel = welcome.ChannelValue(s).ID
		}
	})
	if err != nil {
		return fmt.Errorf("welcome: setup: %w", err)
	}

	msg := describeRole(s, store, e.GuildID, roleID)
	// Warned now rather than discovered when welcoming someone.
	for _, o := range []*discordgo.ApplicationCommandInteractionDataOption{intro, welcome} {
		if o == nil {
			continue
		}
		if why := cannotPost(s, e.GuildID, o.ChannelValue(s).ID); why != "" {
			msg += "\n\n⚠️ " + why
		}
	}
	return respond(s, e, msg)
}

// describeRole is a role's welcome settings in a few lines.
func describeRole(s *discordgo.Session, store *storage.Storage, guildID, roleID string) string {
	w := store.WelcomeRoleFor(guildID, roleID)
	if w == nil {
		return fmt.Sprintf("<@&%s> has no welcome set up. `/welcome setup` picks the channels, `/welcome template` writes the texts.", roleID)
	}
	return fmt.Sprintf("<@&%s>\n%s", roleID, partLines(w))
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
func runTemplate(context *cmdadapter.SlashInteractionContext, opts options) error {
	s, e, store := context.Session, context.Event, context.Storage
	roleID := roleIDOf(opts)
	kind := opts[optKind].StringValue()

	current := ""
	if w := store.WelcomeRoleFor(e.GuildID, roleID); w != nil {
		current = w.IntroTemplate
		if kind == kindWelcome {
			current = w.WelcomeTemplate
		}
	}

	title := "Intro text"
	if kind == kindWelcome {
		title = "Welcome text"
	}
	title += " for " + roleName(s, e.GuildID, roleID)
	if r := []rune(title); len(r) > 45 {
		title = string(r[:45])
	}

	return s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: modalPrefix + kind + ":" + roleID,
			Title:    title,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    modalField,
						Label:       "Text ({user} {name} {server} {role})",
						Style:       discordgo.TextInputParagraph,
						Placeholder: "Paste it as written. #channel-names become links. Empty removes it.",
						Value:       current,
						Required:    false,
						MaxLength:   maxMessage,
					},
				}},
			},
		},
	})
}

// ModalSubmit saves a template written in the editor.
func (c *WelcomeCommand) ModalSubmit(context *cmdadapter.ComponentInteractionContext) error {
	s, e, store := context.Session, context.Event, context.Storage
	if !isAdmin(context) {
		return reply.RespondEphemeral(s, e, "Only administrators can change welcome texts.")
	}

	data := e.ModalSubmitData()
	rest := strings.TrimPrefix(data.CustomID, modalPrefix)
	kind, roleID, ok := strings.Cut(rest, ":")
	if !ok || (kind != kindIntro && kind != kindWelcome) || roleID == "" {
		return reply.RespondEphemeral(s, e, "That editor is out of date. Open it again with `/welcome template`.")
	}

	// Acknowledged before anything else, and answered by editing that in:
	// looking up archived threads is a request per channel, and Discord
	// gives a modal three seconds before it tells the administrator
	// "Something went wrong" — which it did, on a server with a few dozen
	// channels, while the text was being saved behind it.
	if err := reply.RespondDeferredEphemeral(s, e); err != nil {
		return fmt.Errorf("welcome: acknowledge template: %w", err)
	}
	answer := func(msg string) error {
		return reply.EditResponseEmbed(s, e, &discordgo.MessageEmbed{Description: msg, Color: reply.EmbedColor})
	}

	text := strings.TrimSpace(modalValue(data.Components, modalField))
	text = linkArchived(s, e.GuildID, text, guildChannels(s, e.GuildID))
	err := store.UpdateWelcomeRole(e.GuildID, roleID, func(w *storage.WelcomeRole) {
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
	v := Vars{UserID: e.Member.User.ID, Name: displayName(e.Member), Server: guildName(s, e.GuildID), Role: roleName(s, e.GuildID, roleID)}
	rendered := Render(text, v, guildChannels(s, e.GuildID))

	msg := fmt.Sprintf("Saved the %s text for <@&%s>. With you in it, it reads:\n\n%s", kind, roleID, rendered)
	if missing := unlinked(rendered); len(missing) > 0 {
		msg += "\n\n⚠️ No channel or thread matches " + strings.Join(missing, ", ") + unlinkedHint
	}
	if TooLong(rendered) {
		msg += "\n\n⚠️ It is over Discord's 2000 characters with a name in it, and will be refused until shortened."
	}
	return answer(trimEmbed(msg))
}

// modalValue finds a text input's value in a submitted modal.
func modalValue(components []discordgo.MessageComponent, customID string) string {
	for _, c := range components {
		row, ok := c.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, inner := range row.Components {
			if in, ok := inner.(*discordgo.TextInput); ok && in.CustomID == customID {
				return in.Value
			}
		}
	}
	return ""
}

// runPreview shows a role's texts as they would be posted, and where.
func runPreview(context *cmdadapter.SlashInteractionContext, opts options) error {
	s, e, store := context.Session, context.Event, context.Storage
	roleID := roleIDOf(opts)
	w := store.WelcomeRoleFor(e.GuildID, roleID)
	if w == nil {
		return respond(s, e, describeRole(s, store, e.GuildID, roleID))
	}

	member := e.Member
	if o := opts[optUser]; o != nil {
		if u := o.UserValue(s); u != nil {
			if m, err := memberOf(s, e.GuildID, u.ID); err == nil {
				member = m
			}
		}
	}
	v := Vars{UserID: member.User.ID, Name: displayName(member), Server: guildName(s, e.GuildID), Role: roleName(s, e.GuildID, roleID)}
	channels := guildChannels(s, e.GuildID)

	var b strings.Builder
	fmt.Fprintf(&b, "Preview for <@%s> as <@&%s> — nothing is posted.\n", member.User.ID, roleID)
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
	if gifs := store.WelcomeGifs(e.GuildID); len(gifs) > 0 {
		fmt.Fprintf(&b, "\nThe welcome also gets one of %d gifs at random.", len(gifs))
	}
	return respond(s, e, trimEmbed(b.String()))
}

// runRoles lists every role with a welcome, a field each.
func runRoles(context *cmdadapter.SlashInteractionContext) error {
	s, e, store := context.Session, context.Event, context.Storage
	roles := store.WelcomeRoles(e.GuildID)
	if len(roles) == 0 {
		return respond(s, e, "No role has a welcome set up yet. `/welcome setup` is where to start.")
	}
	names := make(map[string]string, len(roles))
	for _, r := range roles {
		names[r.RoleID] = roleLabel(s, e.GuildID, r.RoleID)
	}
	slices.SortFunc(roles, func(a, b storage.WelcomeRole) int {
		return strings.Compare(strings.ToLower(names[a.RoleID]), strings.ToLower(names[b.RoleID]))
	})

	embed := &discordgo.MessageEmbed{Title: "👋 Welcomes", Color: reply.EmbedColor}
	for i, r := range roles {
		// Discord takes 25 fields; the rest are named, not described.
		if i == maxFields-1 && len(roles) > maxFields {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  fmt.Sprintf("…and %d more", len(roles)-i),
				Value: "`/welcome setup role:` shows any one of them.",
			})
			break
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: names[r.RoleID], Value: partLines(&r)})
	}

	gifs := "No gifs — welcomes go out without one"
	if n := len(store.WelcomeGifs(e.GuildID)); n > 0 {
		gifs = fmt.Sprintf("%d gifs to pick welcomes from", n)
	}
	embed.Footer = &discordgo.MessageEmbedFooter{Text: gifs + " · /welcome preview shows a role's texts"}
	return reply.RespondEmbedEphemeral(s, e, embed)
}

// maxFields is how many fields Discord allows in one embed.
const maxFields = 25

// roleLabel is a role's name as plain text, for where Discord does not turn
// a mention into one, such as an embed field's name.
func roleLabel(s *discordgo.Session, guildID, roleID string) string {
	if r, err := s.State.Role(guildID, roleID); err == nil && r != nil {
		return "@" + r.Name
	}
	return "Deleted role " + roleID
}

// runMove gives a role's welcome to another role, so one written and tried
// on a test role and test channels is not written again for the real ones.
func runMove(context *cmdadapter.SlashInteractionContext, opts options) error {
	s, e, store := context.Session, context.Event, context.Storage
	from, _ := opts[optFrom].Value.(string)
	to, _ := opts[optTo].Value.(string)
	keep := opts[optKeep] != nil && opts[optKeep].BoolValue()
	intro, welcome := opts[optIntro], opts[optWelcome]

	switch {
	case from == to:
		return respond(s, e, "Those are the same role.")
	case to == e.GuildID:
		return respond(s, e, "Everyone has @everyone, so it cannot say who is being welcomed. Pick a role.")
	}

	err := store.MoveWelcomeRole(e.GuildID, from, to, keep, func(w *storage.WelcomeRole) {
		if intro != nil {
			w.IntroChannel = intro.ChannelValue(s).ID
		}
		if welcome != nil {
			w.WelcomeChannel = welcome.ChannelValue(s).ID
		}
	})
	switch {
	case errors.Is(err, storage.ErrWelcomeRoleMissing):
		return respond(s, e, fmt.Sprintf("<@&%s> has no welcome to move. `/welcome roles` lists the ones that do.", from))
	case errors.Is(err, storage.ErrWelcomeRoleTaken):
		return respond(s, e, fmt.Sprintf("<@&%s> already has a welcome, and it is not written over. "+
			"`/welcome remove` it first if this one should take its place.", to))
	case err != nil:
		return fmt.Errorf("welcome: move: %w", err)
	}

	verb := "Moved"
	if keep {
		verb = "Copied"
	}
	msg := fmt.Sprintf("%s the welcome from <@&%s> to <@&%s>.\n\n%s", verb, from, to, describeRole(s, store, e.GuildID, to))
	w := store.WelcomeRoleFor(e.GuildID, to)
	for _, ch := range []string{w.IntroChannel, w.WelcomeChannel} {
		if ch == "" {
			continue
		}
		if why := cannotPost(s, e.GuildID, ch); why != "" {
			msg += "\n\n⚠️ " + why
		}
	}
	if intro == nil || welcome == nil {
		msg += "\n\nChannels not given were kept as they were — `/welcome setup` changes them."
	}
	msg += "\n`/welcome preview` shows the texts as they would be posted."
	return respond(s, e, msg)
}

func runRemove(context *cmdadapter.SlashInteractionContext, opts options) error {
	s, e, store := context.Session, context.Event, context.Storage
	roleID := roleIDOf(opts)
	if store.WelcomeRoleFor(e.GuildID, roleID) == nil {
		return respond(s, e, fmt.Sprintf("<@&%s> had no welcome set up.", roleID))
	}
	if err := store.RemoveWelcomeRole(e.GuildID, roleID); err != nil {
		return fmt.Errorf("welcome: remove: %w", err)
	}
	return respond(s, e, fmt.Sprintf("Removed the welcome for <@&%s>. Who was already welcomed is still remembered.", roleID))
}

func runGifs(context *cmdadapter.SlashInteractionContext, sub string, opts options) error {
	s, e, store := context.Session, context.Event, context.Storage

	switch sub {
	case subGifAdd:
		link := strings.TrimSpace(opts[optURL].StringValue())
		if u, err := url.Parse(link); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return respond(s, e, "That is not a link. Paste the gif's address, starting with https://.")
		}
		if err := store.AddWelcomeGif(e.GuildID, link); err != nil {
			if errors.Is(err, storage.ErrWelcomeGifsFull) {
				return respond(s, e, "Not added: "+err.Error()+".")
			}
			return fmt.Errorf("welcome: add gif: %w", err)
		}
		// The page is read for the gif file behind it, which can take
		// longer than Discord waits for an answer.
		if err := reply.RespondDeferredEphemeral(s, e); err != nil {
			return fmt.Errorf("welcome: acknowledge gif: %w", err)
		}
		msg := fmt.Sprintf("Added. Welcomes pick from %d gifs.", len(store.WelcomeGifs(e.GuildID)))
		if media := gifMedia(store, e.GuildID, link); media != "" {
			msg += "\nIt will be posted as the gif itself, without the link."
		} else {
			msg += "\n⚠️ I could not find the gif file behind that link, so it will be posted as the link. " +
				"A link straight to the file (ending in .gif) always works."
		}
		return reply.EditResponseEmbed(s, e, &discordgo.MessageEmbed{Description: msg + "\n" + link, Color: reply.EmbedColor})

	case subGifRemove:
		removed, err := store.RemoveWelcomeGif(e.GuildID, opts[optURL].StringValue())
		if err != nil {
			return fmt.Errorf("welcome: remove gif: %w", err)
		}
		if !removed {
			return respond(s, e, "That link is not in the list. `/welcome gifs` shows what is.")
		}
		return respond(s, e, "Removed.")

	default:
		gifs := store.WelcomeGifs(e.GuildID)
		if len(gifs) == 0 {
			return respond(s, e, "No gifs yet — welcomes go out without one. `/welcome gif-add` adds one.")
		}
		return respond(s, e, trimEmbed(fmt.Sprintf("Welcomes pick one of these at random:\n%s", strings.Join(gifs, "\n"))))
	}
}

// trimEmbed keeps a reply inside an embed's 4096 characters.
func trimEmbed(s string) string {
	if r := []rune(s); len(r) > 3900 {
		return string(r[:3900]) + "…"
	}
	return s
}
