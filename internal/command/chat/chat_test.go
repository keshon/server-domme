package chat

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/storage"
)

// Being switched off and failing to start produce the same silence in a
// channel, and the message is the only thing that tells an administrator which
// one they are looking at.
func TestUnavailableMessageDistinguishesOffFromBroken(t *testing.T) {
	off := unavailableMessage("")
	if !strings.Contains(off, "CHAT_ENABLED") {
		t.Errorf("the switched-off message does not name the setting to change:\n%s", off)
	}

	broken := unavailableMessage("the character file could not be read")
	if strings.Contains(broken, "CHAT_ENABLED") {
		t.Errorf("a start-up failure was reported as the feature being switched off:\n%s", broken)
	}
	if !strings.Contains(broken, "the character file could not be read") {
		t.Errorf("the reason was dropped:\n%s", broken)
	}
}

// The reason is the whole payload. Reporting that something went wrong without
// saying what is what sent an operator to check a setting they had already set.
func TestUnavailableMessageCarriesTheReasonVerbatim(t *testing.T) {
	reason := "no chat backend could be reached at startup"
	got := unavailableMessage(reason)

	if !strings.HasSuffix(strings.TrimSpace(got), reason) {
		t.Errorf("the reason is not carried through intact:\n%s", got)
	}
}

func TestTrimForEmbedFlattensAndCuts(t *testing.T) {
	got := trimForEmbed("line one\n  line two\ttabbed")
	if got != "line one line two tabbed" {
		t.Errorf("trimForEmbed = %q, want it flattened onto one line", got)
	}

	// A relay can answer with a whole HTML error page from a proxy in front of
	// it, which Discord would refuse as an embed.
	long := trimForEmbed(strings.Repeat("x", maxBackendErrorChars*3))
	if len(long) > maxBackendErrorChars+len("…") {
		t.Errorf("trimForEmbed left %d chars, want it cut to %d", len(long), maxBackendErrorChars)
	}
}

func TestEverySubcommandIsOfferedToDiscord(t *testing.T) {
	offered := make(map[string]bool)
	for _, opt := range (&ChatCommand{}).SlashDefinition().Options {
		offered[opt.Name] = true
	}

	for _, name := range []string{subChannel, subBrief, subStatus, subForget, subRole, subAbout, subWhy, subBackends, subReflect} {
		if !offered[name] {
			t.Errorf("%q is handled in Run but never offered in SlashDefinition, so nobody can run it", name)
		}
	}
	if len(offered) != 9 {
		t.Errorf("SlashDefinition offers %d subcommands, want 9 — an unhandled one fails closed", len(offered))
	}
}

// Every offered subcommand needs a description: Discord refuses a command
// definition without one, and the refusal arrives at sync time as a 400 with
// no indication which entry was at fault.
func TestEverySubcommandIsDescribed(t *testing.T) {
	for _, opt := range (&ChatCommand{}).SlashDefinition().Options {
		if strings.TrimSpace(opt.Description) == "" {
			t.Errorf("subcommand %q has no description", opt.Name)
		}
	}
}

// Wiping a character's history cannot be undone and there is no copy, so it
// takes a typed word rather than a click.
func TestForgetIsGuardedByATypedWord(t *testing.T) {
	var confirmOpt *discordgo.ApplicationCommandOption
	for _, opt := range (&ChatCommand{}).SlashDefinition().Options {
		if opt.Name != subForget {
			continue
		}
		for _, inner := range opt.Options {
			if inner.Name == optConfirm {
				confirmOpt = inner
			}
		}
	}

	if confirmOpt == nil {
		t.Fatal("forget has no confirm option, so a stray click wipes the lot")
	}
	if !confirmOpt.Required {
		t.Error("the confirmation is optional, which makes it decoration")
	}
	if confirmOpt.Type != discordgo.ApplicationCommandOptionString {
		t.Errorf("confirm is a %v, want a string the admin has to type", confirmOpt.Type)
	}
}

func TestExplainShowsWhatSheMadeOfIt(t *testing.T) {
	got := explain(storage.MindJournal{
		Username: "Big M", Excerpt: "sorry.. I learned my lesson",
		Trigger: "follow-up", Read: "a sincere apology", Feel: "disarmed",
		Act: "reply", Intent: "accept it without making a thing of it",
		Outcome: "answered", Posted: "fine. we're good",
	})
	for _, want := range []string{"carried on talking to her, untagged", "a sincere apology", "to answer — accept it", "we're good"} {
		if !strings.Contains(got, want) {
			t.Errorf("explanation missing %q:\n%s", want, got)
		}
	}

	got = explain(storage.MindJournal{Username: "Big M", Trigger: "reach", Why: "I wanted to know what he named it", Outcome: "answered"})
	if !strings.Contains(got, "went to **Big M** on her own") || !strings.Contains(got, "what he named it") {
		t.Errorf("a reach does not say why:\n%s", got)
	}
}
