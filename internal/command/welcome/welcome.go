// Package welcome is /welcome: introducing and welcoming a new member the way
// this server does it, on an administrator's say-so.
//
// Not automatic, by request. Roles on this kind of server are picked after
// joining, sometimes changed, and occasionally given by mistake; a person
// deciding "this one is ready" is the guard no event can replace. The command
// then does the fiddly part — right channel, right text for their role, the
// tag, a gif — and refuses rather than guesses whenever something is off.
package welcome

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/perm"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

// Subcommands and options.
const (
	subMember    = "member"
	subSetup     = "setup"
	subTemplate  = "template"
	subPreview   = "preview"
	subRoles     = "roles"
	subRemove    = "remove"
	subGifAdd    = "gif-add"
	subGifRemove = "gif-remove"
	subGifs      = "gifs"

	optUser    = "user"
	optRole    = "role"
	optAgain   = "again"
	optIntro   = "intro_channel"
	optWelcome = "welcome_channel"
	optKind    = "kind"
	optURL     = "url"

	kindIntro   = "intro"
	kindWelcome = "welcome"

	// modalPrefix starts the customID of the template editor, followed by
	// the kind and the role: "welcome:tpl:intro:123".
	modalPrefix = "welcome:tpl:"
	modalField  = "template"
)

// WelcomeCommand introduces and welcomes members.
type WelcomeCommand struct{}

func (c *WelcomeCommand) Name() string { return "welcome" }
func (c *WelcomeCommand) Description() string {
	return "Introduce and welcome a member, the way this server does it"
}
func (c *WelcomeCommand) Group() string    { return "welcome" }
func (c *WelcomeCommand) Category() string { return "👋 Welcome" }

// UserPermissions keeps the whole command with administrators: it posts in
// the server's name and tags people, and its templates are the server's
// first words to every newcomer.
func (c *WelcomeCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func roleOption(required bool, what string) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type: discordgo.ApplicationCommandOptionRole, Name: optRole,
		Description: what, Required: required,
	}
}

func (c *WelcomeCommand) SlashDefinition() *discordgo.ApplicationCommand {
	textChannels := []discordgo.ChannelType{discordgo.ChannelTypeGuildText}
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type: discordgo.ApplicationCommandOptionSubCommand, Name: subMember,
				Description: "Post their role's intro and welcome — checks everything first",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionUser, Name: optUser, Description: "Who to welcome", Required: true},
					roleOption(false, "Which of their roles to welcome them as, if they have more than one"),
					{Type: discordgo.ApplicationCommandOptionBoolean, Name: optAgain, Description: "Post again even if they were already welcomed for this role", Required: false},
				},
			},
			{
				Type: discordgo.ApplicationCommandOptionSubCommand, Name: subSetup,
				Description: "Where a role's intro and welcome go. Leave both empty to see",
				Options: []*discordgo.ApplicationCommandOption{
					roleOption(true, "The role"),
					{Type: discordgo.ApplicationCommandOptionChannel, Name: optIntro, Description: "Where the intro is posted", ChannelTypes: textChannels},
					{Type: discordgo.ApplicationCommandOptionChannel, Name: optWelcome, Description: "Where the welcome is posted", ChannelTypes: textChannels},
				},
			},
			{
				Type: discordgo.ApplicationCommandOptionSubCommand, Name: subTemplate,
				Description: "Write a role's intro or welcome text — paste it straight from Discord",
				Options: []*discordgo.ApplicationCommandOption{
					roleOption(true, "The role"),
					{
						Type: discordgo.ApplicationCommandOptionString, Name: optKind, Description: "Which text", Required: true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "intro", Value: kindIntro},
							{Name: "welcome", Value: kindWelcome},
						},
					},
				},
			},
			{
				Type: discordgo.ApplicationCommandOptionSubCommand, Name: subPreview,
				Description: "See a role's intro and welcome as they would be posted, without posting",
				Options: []*discordgo.ApplicationCommandOption{
					roleOption(true, "The role"),
					{Type: discordgo.ApplicationCommandOptionUser, Name: optUser, Description: "Who to preview it for — you, if empty"},
				},
			},
			{Type: discordgo.ApplicationCommandOptionSubCommand, Name: subRoles, Description: "Every role with a welcome set up"},
			{
				Type: discordgo.ApplicationCommandOptionSubCommand, Name: subRemove,
				Description: "Remove a role's welcome settings",
				Options:     []*discordgo.ApplicationCommandOption{roleOption(true, "The role")},
			},
			{
				Type: discordgo.ApplicationCommandOptionSubCommand, Name: subGifAdd,
				Description: "Add a gif link to pick welcomes from",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionString, Name: optURL, Description: "A link to a gif (tenor, giphy, a .gif)", Required: true},
				},
			},
			{
				Type: discordgo.ApplicationCommandOptionSubCommand, Name: subGifRemove,
				Description: "Remove a gif link",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionString, Name: optURL, Description: "The link to remove", Required: true},
				},
			},
			{Type: discordgo.ApplicationCommandOptionSubCommand, Name: subGifs, Description: "The gifs welcomes pick from"},
		},
	}
}

func (c *WelcomeCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}
	s, e := context.Session, context.Event

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return respond(s, e, "Pick something: `member`, `setup`, `template`, `preview`, `roles`, `remove`, `gif-add`, `gif-remove` or `gifs`.")
	}
	sub := data.Options[0]
	opts := optionsOf(sub)

	switch sub.Name {
	case subMember:
		return runMember(context, opts)
	case subSetup:
		return runSetup(context, opts)
	case subTemplate:
		return runTemplate(context, opts)
	case subPreview:
		return runPreview(context, opts)
	case subRoles:
		return runRoles(context)
	case subRemove:
		return runRemove(context, opts)
	case subGifAdd, subGifRemove, subGifs:
		return runGifs(context, sub.Name, opts)
	default:
		return respond(s, e, "Unknown subcommand: "+sub.Name)
	}
}

func optionsOf(sub *discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	out := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(sub.Options))
	for _, o := range sub.Options {
		out[o.Name] = o
	}
	return out
}

func respond(s *discordgo.Session, e *discordgo.InteractionCreate, msg string) error {
	return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{Description: msg, Color: reply.EmbedColor})
}

// runMember welcomes one person. Every check runs before anything is posted,
// and each part — intro, welcome — is checked, posted and recorded on its own,
// so a run that fails half way can be repeated to finish the other half
// without posting the first half twice.
func runMember(context *cmdadapter.SlashInteractionContext, opts map[string]*discordgo.ApplicationCommandInteractionDataOption) error {
	s, e, store := context.Session, context.Event, context.Storage

	// Fetching the member and posting two messages can pass Discord's three
	// seconds to answer an interaction; acknowledge first.
	if err := reply.RespondDeferredEphemeral(s, e); err != nil {
		return err
	}
	finish := func(msg string) error {
		return reply.EditResponseEmbed(s, e, &discordgo.MessageEmbed{Description: msg, Color: reply.EmbedColor})
	}

	user := opts[optUser].UserValue(s)
	if user == nil {
		return finish("I could not find that user.")
	}
	if user.Bot {
		return finish("That is a bot. Bots do not get welcomed.")
	}
	member, err := memberOf(s, e.GuildID, user.ID)
	if err != nil {
		return finish(fmt.Sprintf("<@%s> is not in this server, so there is nobody to welcome.", user.ID))
	}

	roleID, problem := pickRole(store, e.GuildID, member, opts[optRole])
	if problem != "" {
		return finish(problem)
	}
	cfg := store.WelcomeRoleFor(e.GuildID, roleID)

	v := Vars{UserID: user.ID, Name: displayName(member), Server: guildName(s, e.GuildID), Role: roleName(s, e.GuildID, roleID)}
	channels := guildChannels(s, e.GuildID)
	again := opts[optAgain] != nil && opts[optAgain].BoolValue()
	done := store.WelcomedFor(e.GuildID, user.ID, roleID)

	intro := planPart(s, e.GuildID, "Intro", cfg.IntroChannel, cfg.IntroTemplate, v, channels, "")
	welcome := planPart(s, e.GuildID, "Welcome", cfg.WelcomeChannel, cfg.WelcomeTemplate, v, channels, randomGif(store.WelcomeGifs(e.GuildID)))
	if done != nil && !again {
		if !done.IntroAt.IsZero() && intro.ok() {
			intro.skip = fmt.Sprintf("already posted <t:%d:R> by <@%s> — add `again:True` to post it again", done.IntroAt.Unix(), done.By)
		}
		if !done.WelcomeAt.IsZero() && welcome.ok() {
			welcome.skip = fmt.Sprintf("already posted <t:%d:R> by <@%s> — add `again:True` to post it again", done.WelcomeAt.Unix(), done.By)
		}
	}

	if !intro.ok() && !welcome.ok() {
		return finish(fmt.Sprintf("Nothing was posted for <@%s> as <@&%s>.\n\n%s\n%s", user.ID, roleID, intro.report(), welcome.report()))
	}

	now := time.Now()
	introSent := intro.send(s, e.GuildID, user.ID)
	welcomeSent := welcome.send(s, e.GuildID, user.ID)
	if introSent || welcomeSent {
		if err := store.MarkWelcomed(e.GuildID, user.ID, roleID, e.Member.User.ID, introSent, welcomeSent, now); err != nil {
			context.AppLog.Warn().Err(err).Str("guild_id", e.GuildID).Msg("welcome_record_failed")
		}
	}

	return finish(fmt.Sprintf("**<@%s> as <@&%s>**\n%s\n%s", user.ID, roleID, intro.report(), welcome.report()))
}

// pickRole decides which of someone's roles to welcome them as, or explains
// why it cannot.
func pickRole(store *storage.Storage, guildID string, member *discordgo.Member, opt *discordgo.ApplicationCommandInteractionDataOption) (string, string) {
	configured := store.WelcomeRoles(guildID)
	if len(configured) == 0 {
		return "", "No role has a welcome set up yet. Start with `/welcome setup` and `/welcome template`."
	}

	if opt != nil {
		roleID, _ := opt.Value.(string)
		if store.WelcomeRoleFor(guildID, roleID) == nil {
			return "", fmt.Sprintf("<@&%s> has no welcome set up. `/welcome roles` lists the ones that do.", roleID)
		}
		// The role is the reason for everything posted, so they have to
		// actually have it: welcoming someone as a Domme who is not one is
		// the mistake this command most needs to refuse.
		if !slices.Contains(member.Roles, roleID) {
			return "", fmt.Sprintf("<@%s> does not have <@&%s>. Give them the role first, then welcome them.", member.User.ID, roleID)
		}
		return roleID, ""
	}

	var theirs []string
	for _, w := range configured {
		if slices.Contains(member.Roles, w.RoleID) {
			theirs = append(theirs, w.RoleID)
		}
	}
	switch len(theirs) {
	case 1:
		return theirs[0], ""
	case 0:
		return "", fmt.Sprintf("<@%s> has none of the roles with a welcome set up (%s). Give them one first.",
			member.User.ID, mentionRoles(configured))
	default:
		return "", fmt.Sprintf("<@%s> has more than one role with a welcome: %s. Say which with `role:`.",
			member.User.ID, mentionRoleIDs(theirs))
	}
}

// part is one message to post: checked and rendered before anything is sent.
type part struct {
	label     string
	channelID string
	content   string
	gif       string
	// problem is why it cannot be posted, skip why it will not be, and
	// posted the link to it once it has been.
	problem, skip, posted string
}

func (p *part) ok() bool { return p.problem == "" && p.skip == "" }

func (p *part) report() string {
	switch {
	case p.posted != "":
		return fmt.Sprintf("✅ **%s** posted in <#%s> — %s", p.label, p.channelID, p.posted)
	case p.problem != "":
		return fmt.Sprintf("⚠️ **%s** not posted: %s", p.label, p.problem)
	case p.skip != "":
		return fmt.Sprintf("➖ **%s** %s", p.label, p.skip)
	default:
		return fmt.Sprintf("⚠️ **%s** not posted", p.label)
	}
}

// planPart checks and renders one part without sending it.
func planPart(s *discordgo.Session, guildID, label, channelID, template string, v Vars, channels []Channel, gif string) *part {
	p := &part{label: label, channelID: channelID, gif: gif}
	switch {
	case strings.TrimSpace(template) == "" && channelID == "":
		p.skip = "is not set up for this role"
		return p
	case strings.TrimSpace(template) == "":
		p.problem = "no text written yet — `/welcome template`"
		return p
	case channelID == "":
		p.problem = "no channel set — `/welcome setup`"
		return p
	}
	if why := cannotPost(s, guildID, channelID); why != "" {
		p.problem = why
		return p
	}
	p.content = Render(template, v, channels)
	if TooLong(p.content) {
		p.problem = fmt.Sprintf("the text comes to %d characters with their name in it, over Discord's 2000", len([]rune(p.content)))
	}
	return p
}

// send posts a planned part, and its gif after it when the two will not fit
// in one message. It reports whether the part went out.
func (p *part) send(s *discordgo.Session, guildID, userID string) bool {
	if !p.ok() {
		return false
	}
	content := p.content
	separateGif := false
	if p.gif != "" {
		if joined := content + "\n" + p.gif; !TooLong(joined) {
			content = joined
		} else {
			separateGif = true
		}
	}

	msg, err := s.ChannelMessageSendComplex(p.channelID, &discordgo.MessageSend{
		Content: content,
		// Only the person being welcomed is notified, whatever the text
		// says. A template with a role mention or @everyone in it would
		// otherwise ping a whole server for one newcomer.
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
			Users: []string{userID},
		},
	})
	if err != nil {
		p.problem = "Discord refused it: " + err.Error()
		return false
	}
	if separateGif {
		_, _ = s.ChannelMessageSend(p.channelID, p.gif)
	}
	p.posted = fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guildID, p.channelID, msg.ID)
	return true
}

// cannotPost reports why the bot could not post in a channel, or "".
func cannotPost(s *discordgo.Session, guildID, channelID string) string {
	ch, err := s.State.Channel(channelID)
	if err != nil || ch == nil {
		ch, err = s.Channel(channelID)
	}
	if err != nil || ch == nil {
		return fmt.Sprintf("<#%s> no longer exists", channelID)
	}
	if ch.GuildID != guildID {
		return fmt.Sprintf("<#%s> is not in this server", channelID)
	}
	if s.State.User == nil {
		return ""
	}
	perms, err := s.State.UserChannelPermissions(s.State.User.ID, channelID)
	if err != nil {
		// Permissions could not be worked out from the cache; let Discord
		// be the judge rather than refusing on a guess.
		return ""
	}
	need := int64(discordgo.PermissionViewChannel | discordgo.PermissionSendMessages)
	if perms&need != need {
		return fmt.Sprintf("I cannot post in <#%s> — give me View Channel and Send Messages there", channelID)
	}
	return ""
}

func memberOf(s *discordgo.Session, guildID, userID string) (*discordgo.Member, error) {
	if m, err := s.State.Member(guildID, userID); err == nil && m != nil {
		return m, nil
	}
	return s.GuildMember(guildID, userID)
}

func displayName(m *discordgo.Member) string {
	switch {
	case m.Nick != "":
		return m.Nick
	case m.User != nil && m.User.GlobalName != "":
		return m.User.GlobalName
	case m.User != nil:
		return m.User.Username
	default:
		return "there"
	}
}

func guildName(s *discordgo.Session, guildID string) string {
	if g, err := s.State.Guild(guildID); err == nil && g != nil {
		return g.Name
	}
	return "the server"
}

func roleName(s *discordgo.Session, guildID, roleID string) string {
	if r, err := s.State.Role(guildID, roleID); err == nil && r != nil {
		return r.Name
	}
	return "member"
}

// guildChannels are the channels and threads a template may name. Threads
// are kept apart from channels in Discord's guild state, and a thread's name
// can have spaces in it ("#Domme Icons Full List"); Render matches whole
// names, longest first, so the spaces need nothing special once the thread
// is in the list. Only threads Discord still has open are there: an archived
// one is not in the state, and is linked by pasting its link instead.
func guildChannels(s *discordgo.Session, guildID string) []Channel {
	g, err := s.State.Guild(guildID)
	if err != nil || g == nil {
		return nil
	}
	out := make([]Channel, 0, len(g.Channels)+len(g.Threads))
	for _, c := range g.Channels {
		out = append(out, Channel{ID: c.ID, Name: c.Name})
	}
	for _, t := range g.Threads {
		out = append(out, Channel{ID: t.ID, Name: t.Name})
	}
	return out
}

// unlinkedHint is said after names that matched nothing.
const unlinkedHint = " — those stay plain text. Archived threads are looked up when a text is " +
	"saved; if one is still not found, paste the thread's link into the template, and Discord shows it as a link."

// linkArchived writes links to archived threads into a template being
// saved, for "#names" nothing open matches.
//
// Discord keeps only open threads in the state, and a thread goes quiet and
// is archived after a few days — the kind of thread a welcome points at, a
// list or a guide, most of all. Looking them up costs a request per channel,
// which is fine once when an administrator saves a text and not fine every
// time someone joins. So it is done here, and what it finds is written into
// the saved text as "<#id>", which Discord links whether the thread is open
// or not. Names that are also a channel's or an open thread's are left to
// Render, as before, and placeholders are left as they are.
func linkArchived(s *discordgo.Session, guildID, text string, known []Channel) string {
	if len(unlinked(Render(text, Vars{}, known))) == 0 {
		return text
	}
	taken := make(map[string]bool, len(known))
	for _, c := range known {
		taken[strings.ToLower(c.Name)] = true
	}
	var archived []Channel
	for _, t := range archivedThreads(s, guildID) {
		if !taken[strings.ToLower(t.Name)] {
			archived = append(archived, t)
		}
	}
	if len(archived) == 0 {
		return text
	}
	return linkChannels(invisible.Replace(text), archived)
}

// archivedThreads are the public archived threads under the guild's
// channels, the most recent hundred of each. A channel the bot cannot read
// is skipped.
func archivedThreads(s *discordgo.Session, guildID string) []Channel {
	g, err := s.State.Guild(guildID)
	if err != nil || g == nil {
		return nil
	}
	var out []Channel
	for _, c := range g.Channels {
		switch c.Type {
		case discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews, discordgo.ChannelTypeGuildForum, discordgo.ChannelTypeGuildMedia:
		default:
			continue
		}
		list, err := s.ThreadsArchived(c.ID, nil, 100)
		if err != nil || list == nil {
			continue
		}
		for _, t := range list.Threads {
			out = append(out, Channel{ID: t.ID, Name: t.Name})
		}
	}
	return out
}

func randomGif(gifs []string) string {
	if len(gifs) == 0 {
		return ""
	}
	return gifs[rand.IntN(len(gifs))]
}

func mentionRoles(roles []storage.WelcomeRole) string {
	ids := make([]string, 0, len(roles))
	for _, r := range roles {
		ids = append(ids, r.RoleID)
	}
	return mentionRoleIDs(ids)
}

func mentionRoleIDs(ids []string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, "<@&"+id+">")
	}
	return strings.Join(parts, ", ")
}

// unlinked finds "#names" a rendered text still carries, which match no
// channel — a renamed channel, or a typo — so they can be flagged before the
// text is ever posted.
var unlinkedChannel = regexp.MustCompile(`(?:^|\s)#([^\s#<>.,!?;:]+)`)

// A name that matches nothing has no known end: "#Domme Icons Full List"
// might be a thread called "Domme" followed by words. So only its first word
// is quoted, with "…" when another word follows, so the partial name does not
// read as the whole of it.
func unlinked(rendered string) []string {
	var out []string
	for _, m := range unlinkedChannel.FindAllStringSubmatchIndex(rendered, -1) {
		name := "#" + rendered[m[2]:m[3]]
		if rest := rendered[m[3]:]; strings.HasPrefix(rest, " ") {
			if next, _ := utf8.DecodeRuneInString(strings.TrimLeft(rest, " ")); isNameRune(next) {
				name += "…"
			}
		}
		out = append(out, name)
	}
	return out
}

// isAdmin re-checks a modal submitter: submissions arrive without the
// command's permission check, which only ran when the modal was opened.
func isAdmin(context *cmdadapter.ComponentInteractionContext) bool {
	e := context.Event
	return e.Member != nil && perm.IsAdministrator(context.Session, e.GuildID, e.Member, context.Config)
}
