package storage

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestWelcomeRolesAreKeptPerRole(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpdateWelcomeRole("g", "sub", func(w *WelcomeRole) { w.IntroChannel = "mousey" }); err != nil {
		t.Fatalf("UpdateWelcomeRole: %v", err)
	}
	if err := s.UpdateWelcomeRole("g", "sub", func(w *WelcomeRole) { w.IntroTemplate = "hi {user}" }); err != nil {
		t.Fatalf("UpdateWelcomeRole: %v", err)
	}
	if err := s.UpdateWelcomeRole("g", "domme", func(w *WelcomeRole) { w.IntroChannel = "domme-chat" }); err != nil {
		t.Fatalf("UpdateWelcomeRole: %v", err)
	}

	sub := s.WelcomeRoleFor("g", "sub")
	if sub == nil || sub.IntroChannel != "mousey" || sub.IntroTemplate != "hi {user}" {
		t.Errorf("sub settings = %+v, want both updates kept", sub)
	}
	if len(s.WelcomeRoles("g")) != 2 || len(s.WelcomeRoles("other")) != 0 {
		t.Error("roles leaked between guilds or went missing")
	}

	if err := s.RemoveWelcomeRole("g", "sub"); err != nil {
		t.Fatalf("RemoveWelcomeRole: %v", err)
	}
	if s.WelcomeRoleFor("g", "sub") != nil {
		t.Error("removed role still has settings")
	}
}

// Recorded part by part, so a run that failed half way can finish the other
// half without posting the first half twice.
func TestMarkWelcomedKeepsEachPart(t *testing.T) {
	s := newTestStore(t)
	first := time.Now().Add(-time.Hour)
	if err := s.MarkWelcomed("g", "u", "sub", "duchess", true, false, first); err != nil {
		t.Fatalf("MarkWelcomed: %v", err)
	}
	if err := s.MarkWelcomed("g", "u", "sub", "duchess", false, true, time.Now()); err != nil {
		t.Fatalf("MarkWelcomed: %v", err)
	}
	w := s.WelcomedFor("g", "u", "sub")
	if w == nil || !w.IntroAt.Equal(first) || w.WelcomeAt.IsZero() {
		t.Errorf("record = %+v, want the intro's time kept and the welcome added", w)
	}
	if s.WelcomedFor("g", "u", "domme") != nil {
		t.Error("a welcome for one role counted for another")
	}
}

func TestWelcomeGifsAreBoundedAndDeduplicated(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < maxWelcomeGifs; i++ {
		if err := s.AddWelcomeGif("g", fmt.Sprintf("https://gif/%d", i)); err != nil {
			t.Fatalf("AddWelcomeGif: %v", err)
		}
	}
	if err := s.AddWelcomeGif("g", "https://gif/0"); err != nil {
		t.Errorf("re-adding a gif already there failed: %v", err)
	}
	if err := s.AddWelcomeGif("g", "https://gif/new"); !errors.Is(err, ErrWelcomeGifsFull) {
		t.Errorf("adding past the limit: %v", err)
	}
	if removed, err := s.RemoveWelcomeGif("g", "https://gif/3"); err != nil || !removed {
		t.Errorf("RemoveWelcomeGif = %v, %v", removed, err)
	}
	if removed, _ := s.RemoveWelcomeGif("g", "https://gif/3"); removed {
		t.Error("removed a gif twice")
	}
}

// A welcome drafted on a test role is carried over whole, and never over a
// role that already has one.
func TestMoveWelcomeRoleCarriesTheDraftOver(t *testing.T) {
	s := newTestStore(t)
	draft := func(w *WelcomeRole) {
		w.IntroChannel, w.IntroTemplate = "test-intro", "hi {user}"
		w.WelcomeChannel, w.WelcomeTemplate = "test-welcome", "welcome {user}"
	}
	if err := s.UpdateWelcomeRole("g", "test", draft); err != nil {
		t.Fatal(err)
	}

	if err := s.MoveWelcomeRole("g", "test", "sub", false, func(w *WelcomeRole) { w.IntroChannel = "intro" }); err != nil {
		t.Fatalf("MoveWelcomeRole: %v", err)
	}
	sub := s.WelcomeRoleFor("g", "sub")
	if sub == nil || sub.RoleID != "sub" || sub.IntroChannel != "intro" || sub.IntroTemplate != "hi {user}" ||
		sub.WelcomeChannel != "test-welcome" || sub.WelcomeTemplate != "welcome {user}" {
		t.Errorf("moved = %+v, want the draft with the new intro channel", sub)
	}
	if s.WelcomeRoleFor("g", "test") != nil {
		t.Error("a move left the first role's welcome behind")
	}

	if err := s.MoveWelcomeRole("g", "sub", "domme", true, nil); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if s.WelcomeRoleFor("g", "sub") == nil || s.WelcomeRoleFor("g", "domme") == nil {
		t.Error("a copy should leave both roles with a welcome")
	}

	if err := s.MoveWelcomeRole("g", "sub", "domme", false, nil); !errors.Is(err, ErrWelcomeRoleTaken) {
		t.Errorf("move onto a role with a welcome: err = %v, want ErrWelcomeRoleTaken", err)
	}
	if err := s.MoveWelcomeRole("g", "test", "other", false, nil); !errors.Is(err, ErrWelcomeRoleMissing) {
		t.Errorf("move from a role without one: err = %v, want ErrWelcomeRoleMissing", err)
	}
}
