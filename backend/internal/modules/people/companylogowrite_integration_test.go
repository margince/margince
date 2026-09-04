// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A person setting their own company's mark, and taking it off again.
//
// Both writes go through orgLogoWrite, the one statement four logo writers
// share, and the CLEAR is that statement with two NULL values rather than a
// spelling of its own. That is the half worth a test: the set arm is exercised
// by the resolve path's own suite, and the clear arm reaches the same statement
// through a different pair of arguments.
//
// The object key each write hands back is what makes the bytes collectable, so
// it is asserted as carefully as the column: a write that superseded an object
// and reported nothing would leave it in the store for good.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestTheCompanyMarkIsSetAndTakenOffThroughOneStatement(t *testing.T) {
	e := newAnchorEnv(t)

	superseded, err := e.store.SetCompanyLogo(e.ctx, "logos/first.png", "first.png")
	if err != nil {
		t.Fatalf("setting the company mark: %v", err)
	}
	if superseded != nil {
		t.Errorf("the first mark superseded %q — there was nothing to supersede", *superseded)
	}
	assertCompanyMark(t, e, "logos/first.png", "first.png")

	// A second set hands back the first object, which is the only signal the
	// caller has that its bytes are now unreferenced.
	superseded, err = e.store.SetCompanyLogo(e.ctx, "logos/second.png", "second.png")
	if err != nil {
		t.Fatalf("replacing the company mark: %v", err)
	}
	if superseded == nil || *superseded != "logos/first.png" {
		t.Errorf("replacing the mark handed back %v, want the first object", superseded)
	}
	assertCompanyMark(t, e, "logos/second.png", "second.png")

	removed, err := e.store.ClearCompanyLogo(e.ctx)
	if err != nil {
		t.Fatalf("taking the company mark off: %v", err)
	}
	if removed == nil || *removed != "logos/second.png" {
		t.Errorf("clearing the mark handed back %v, want the standing object", removed)
	}
	assertCompanyMark(t, e, "", "")

	// Clearing a company that wears nothing is not an error and collects
	// nothing: the caller's request is already true.
	if again, clearErr := e.store.ClearCompanyLogo(e.ctx); clearErr != nil || again != nil {
		t.Errorf("clearing an unmarked company: got %v %v, want no object and no error", again, clearErr)
	}
}

// assertCompanyMark reads the two columns off the row rather than through the
// company read, because what this is about is what the statement wrote.
func assertCompanyMark(t *testing.T, e *anchorEnv, wantKey, wantOrigin string) {
	t.Helper()
	var key, origin *string
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT logo_object_key, logo_origin FROM organization WHERE id = $1`, e.anchorID).
			Scan(&key, &origin)
	}); err != nil {
		t.Fatalf("reading the mark back: %v", err)
	}
	if derefOrEmpty(key) != wantKey || derefOrEmpty(origin) != wantOrigin {
		t.Errorf("the row carries key %q origin %q, want %q and %q",
			derefOrEmpty(key), derefOrEmpty(origin), wantKey, wantOrigin)
	}
}
