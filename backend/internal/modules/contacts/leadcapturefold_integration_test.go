// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The precedence rule an approved capture-collision card applies: a captured
// value FILLS an empty field and never replaces one a human typed.
//
// Integration, because the rule is decided from a read taken under a row lock
// inside the caller's transaction — a unit test can see the comparison but not
// the ordering that makes it safe.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// foldCaptured runs the fold the way the approved card's effect runs it: the
// custom-field catalog read outside the transaction, the write inside one.
func (e *dedupeEnv) foldCaptured(ctx context.Context, t *testing.T, lead ids.LeadID, in CapturedLeadFields) {
	t.Helper()
	active, err := e.store.ActiveLeadColumns(ctx)
	if err != nil {
		t.Fatalf("reading the active lead columns: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return e.store.FillEmptyLeadFieldsTx(ctx, tx, lead, in, active)
	}); err != nil {
		t.Fatalf("folding the captured fields: %v", err)
	}
}

func (e *dedupeEnv) leadFields(ctx context.Context, t *testing.T, lead ids.LeadID) (name, company, title string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(full_name, ''), coalesce(company_name, ''), coalesce(title, '')
               FROM lead WHERE id = $1`, lead).Scan(&name, &company, &title)
	}); err != nil {
		t.Fatalf("reading the lead back: %v", err)
	}
	return name, company, title
}

// leadVersion reads the optimistic-concurrency column staged proposals pin.
func (e *dedupeEnv) leadVersion(ctx context.Context, t *testing.T, lead ids.LeadID) int64 {
	t.Helper()
	var version int64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT version FROM lead WHERE id = $1`, lead).Scan(&version)
	}); err != nil {
		t.Fatalf("reading the lead's version: %v", err)
	}
	return version
}

// Both halves of the rule in ONE fold, because the failure mode is a patch that
// treats every field alike: a fold onto empty fields only would pass against a
// writer that overwrote everything.
func TestAnApprovedCaptureFoldFillsWhatIsEmptyAndKeepsWhatWasTyped(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead := e.createLead(ctx, t, "Jonas Petersen", "jonas@nordwind.test", "")

	e.foldCaptured(ctx, t, lead, CapturedLeadFields{
		FullName:    "J. Petersen",
		CompanyName: "Nordwind Logistik",
		Title:       "Head of Operations",
	})

	name, company, title := e.leadFields(ctx, t, lead)
	if name != "Jonas Petersen" {
		t.Errorf("the lead is now called %q — a captured value overwrote a name somebody typed, "+
			"which is the one thing this fold must never do", name)
	}
	if company != "Nordwind Logistik" {
		t.Errorf("company_name = %q, want the captured value: the field was empty, and filling it is "+
			"what makes the card worth a human's decision", company)
	}
	if title != "Head of Operations" {
		t.Errorf("title = %q, want the captured value", title)
	}
}

// A message that knows nothing the lead does not know writes nothing at all.
//
// Its own case because "changed no field" has to reach the row as no write, not
// as a write of the same values: a version bump expires every other proposal
// pinned to this lead, and a card that moved no data would have spent them.
func TestACaptureFoldThatFillsNothingLeavesTheLeadAlone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead := e.createLead(ctx, t, "Jonas Petersen", "jonas@nordwind.test", "Nordwind Logistik")
	before := e.leadVersion(ctx, t, lead)

	e.foldCaptured(ctx, t, lead, CapturedLeadFields{
		FullName:    "J. Petersen",
		CompanyName: "Nordwind GmbH",
	})

	name, company, _ := e.leadFields(ctx, t, lead)
	if name != "Jonas Petersen" || company != "Nordwind Logistik" {
		t.Errorf("the lead reads %q / %q, want both of the human's values untouched", name, company)
	}
	if after := e.leadVersion(ctx, t, lead); after != before {
		t.Errorf("the lead's version moved %d → %d for a fold that changed no field; every proposal "+
			"pinned to this row would expire for a decision that wrote nothing", before, after)
	}
}
