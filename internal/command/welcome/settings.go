package welcome

import (
	"errors"
	"fmt"
	"net/url"
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
	line := func(label, channel, template string) string {
		where := "no channel"
		if channel != "" {
			where = "<#" + channel + ">"
		}
		text := "no text yet"
		if strings.TrimSpace(template) != "" {
			text = fmt.Sprintf("%d characters of text", len([]rune(template)))
		}
		return fmt.Sprintf("**%s** → %s, %s", label, where, text)
	}
	return fmt.Sprintf("<@&%s>\n%s\n%s", roleID,
		line("Intro", w.IntroChannel, w.IntroTemplate),
		line("Welcome", w.WelcomeChannel, w.WelcomeTemplate))
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

	text := strings.TrimSpace(modalValue(data.Components, modalField))
	err := store.UpdateWelcomeRole(e.GuildID, roleID, func(w *storage.WelcomeRole) {
		if kind == kindIntro {
			w.IntroTemplate = text
		} else {
			w.WelcomeTemplate = text
		}
	})
	if err != nil {
		return fmt.Errorf("welcome: save template: %w", err)
	}
	if text == "" {
		return respond(s, e, fmt.Sprintf("Removed the %s text for <@&%s>.", kind, roleID))
	}

	// Rendered for the person saving it, so they see the links and their
	// own name where a newcomer's will go.
	v := Vars{UserID: e.Member.User.ID, Name: displayName(e.Member), Server: guildName(s, e.GuildID), Role: roleName(s, e.GuildID, roleID)}
	rendered := Render(text, v, guildChannels(s, e.GuildID))

	msg := fmt.Sprintf("Saved the %s text for <@&%s>. With you in it, it reads:\n\n%s", kind, roleID, rendered)
	if missing := unlinked(rendered); len(missing) > 0 {
		msg += "\n\n⚠️ No channel matches " + strings.Join(missing, ", ") + " — those stay plain text."
	}
	if TooLong(rendered) {
		msg += "\n\n⚠️ It is over Discord's 2000 characters with a name in it, and will be refused until shortened."
	}
	return respond(s, e, trimEmbed(msg))
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
			b.WriteString("⚠️ No channel matches " + strings.Join(missing, ", ") + "\n")
		}
	}
	if gifs := store.WelcomeGifs(e.GuildID); len(gifs) > 0 {
		fmt.Fprintf(&b, "\nThe welcome also gets one of %d gifs at random.", len(gifs))
	}
	return respond(s, e, trimEmbed(b.String()))
}

// runRoles lists every role with a welcome.
func runRoles(context *cmdadapter.SlashInteractionContext) error {
	s, e, store := context.Session, context.Event, context.Storage
	roles := store.WelcomeRoles(e.GuildID)
	if len(roles) == 0 {
		return respond(s, e, "No role has a welcome set up yet. `/welcome setup` is where to start.")
	}
	parts := make([]string, 0, len(roles))
	for _, r := range roles {
		parts = append(parts, describeRole(s, store, e.GuildID, r.RoleID))
	}
	parts = append(parts, fmt.Sprintf("%d gifs to pick welcomes from.", len(store.WelcomeGifs(e.GuildID))))
	return respond(s, e, trimEmbed(strings.Join(parts, "\n\n")))
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
		return respond(s, e, fmt.Sprintf("Added. Welcomes pick from %d gifs.\n%s", len(store.WelcomeGifs(e.GuildID)), link))

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
