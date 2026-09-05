// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A person setting their own company's marks, and taking them off again.
//
// Both writes go through the slot's own spelling of one statement shape, and
// the CLEAR is that statement with two NULL values rather than a spelling of
// its own. That is the half worth a test: the set arm is exercised by the
// resolve path's own suite, and the clear arm reaches the same statement
// through a different pair of arguments.
//
// The object key each write hands back is what makes the bytes collectable, so
// it is asserted as carefully as the column: a write that superseded an object
// and reported nothing would leave it in the store for good.

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestTheCompanyMarkIsSetAndTakenOffThroughOneStatement(t *testing.T) {
	e := newAnchorEnv(t)

	superseded, err := e.store.SetCompanyLogo(e.ctx, LogoWide, "logos/first.png", "first.png")
	if err != nil {
		t.Fatalf("setting the company mark: %v", err)
	}
	if superseded != nil {
		t.Errorf("the first mark superseded %q — there was nothing to supersede", *superseded)
	}
	assertCompanyMark(t, e, "logos/first.png", "first.png")

	// A second set hands back the first object, which is the only signal the
	// caller has that its bytes are now unreferenced.
	superseded, err = e.store.SetCompanyLogo(e.ctx, LogoWide, "logos/second.png", "second.png")
	if err != nil {
		t.Fatalf("replacing the company mark: %v", err)
	}
	if superseded == nil || *superseded != "logos/first.png" {
		t.Errorf("replacing the mark handed back %v, want the first object", superseded)
	}
	assertCompanyMark(t, e, "logos/second.png", "second.png")

	removed, err := e.store.ClearCompanyLogo(e.ctx, LogoWide)
	if err != nil {
		t.Fatalf("taking the company mark off: %v", err)
	}
	if removed == nil || *removed != "logos/second.png" {
		t.Errorf("clearing the mark handed back %v, want the standing object", removed)
	}
	assertCompanyMark(t, e, "", "")

	// Clearing a company that wears nothing is not an error and collects
	// nothing: the caller's request is already true.
	if again, clearErr := e.store.ClearCompanyLogo(e.ctx, LogoWide); clearErr != nil || again != nil {
		t.Errorf("clearing an unmarked company: got %v %v, want no object and no error", again, clearErr)
	}
}

// The two slots are one row and one statement shape, which is exactly the
// arrangement in which a wrong column name is invisible: every assertion about
// the wide mark still passes while the icon writes over it. So each is written,
// read and cleared while the other is watched — a company whose badge upload
// took its wordmark away would be a mark a person did not ask to lose, and no
// test of one slot alone can see it.
func TestTheTwoCompanyMarksAreWrittenAndClearedIndependently(t *testing.T) {
	e := newAnchorEnv(t)

	if _, err := e.store.SetCompanyLogo(e.ctx, LogoWide, "logos/wide.png", "wide.png"); err != nil {
		t.Fatalf("setting the wide mark: %v", err)
	}
	if _, err := e.store.SetCompanyLogo(e.ctx, LogoIcon, "logos/icon.png", "icon.png"); err != nil {
		t.Fatalf("setting the icon: %v", err)
	}
	assertCompanyMark(t, e, "logos/wide.png", "wide.png")
	assertCompanyIcon(t, e, "logos/icon.png", "icon.png")

	// Each slot reads back through the same store verb the streaming endpoint
	// calls, so a slot whose read named the other's column would be caught here
	// rather than by a reader seeing the wrong picture.
	for _, want := range []struct {
		slot LogoSlot
		key  string
	}{{LogoWide, "logos/wide.png"}, {LogoIcon, "logos/icon.png"}} {
		got, err := e.store.OrganizationLogoKey(e.ctx, e.anchorID, want.slot)
		if err != nil {
			t.Fatalf("reading slot %d back: %v", want.slot, err)
		}
		if got != want.key {
			t.Errorf("slot %d reads key %q, want %q", want.slot, got, want.key)
		}
	}

	removed, err := e.store.ClearCompanyLogo(e.ctx, LogoIcon)
	if err != nil {
		t.Fatalf("taking the icon off: %v", err)
	}
	if removed == nil || *removed != "logos/icon.png" {
		t.Errorf("clearing the icon handed back %v, want the icon's object", removed)
	}
	assertCompanyIcon(t, e, "", "")
	// The whole point of the case: the mark the person did NOT touch is still on
	// the row, and still collectable by its own key.
	assertCompanyMark(t, e, "logos/wide.png", "wide.png")
}

// assertCompanyMark reads the two columns off the row rather than through the
// company read, because what this is about is what the statement wrote.
func assertCompanyMark(t *testing.T, e *anchorEnv, wantKey, wantOrigin string) {
	t.Helper()
	assertMarkColumns(t, e, "logo_object_key", "logo_origin", wantKey, wantOrigin)
}

func assertCompanyIcon(t *testing.T, e *anchorEnv, wantKey, wantOrigin string) {
	t.Helper()
	assertMarkColumns(t, e, "logo_icon_object_key", "logo_icon_origin", wantKey, wantOrigin)
}

func assertMarkColumns(t *testing.T, e *anchorEnv, keyColumn, originColumn, wantKey, wantOrigin string) {
	t.Helper()
	var key, origin *string
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		// The column names are this test's own literals, spelled here rather
		// than taken from the slot table: a test that read its expectation out
		// of the code under test would agree with it about a column neither of
		// them names correctly.
		return tx.QueryRow(context.Background(),
			fmt.Sprintf(`SELECT %s, %s FROM organization WHERE id = $1`, keyColumn, originColumn),
			e.anchorID).Scan(&key, &origin)
	}); err != nil {
		t.Fatalf("reading the mark back: %v", err)
	}
	if derefOrEmpty(key) != wantKey || derefOrEmpty(origin) != wantOrigin {
		t.Errorf("%s carries key %q origin %q, want %q and %q",
			keyColumn, derefOrEmpty(key), derefOrEmpty(origin), wantKey, wantOrigin)
	}
}
