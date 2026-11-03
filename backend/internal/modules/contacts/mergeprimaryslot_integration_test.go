// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A merge archives an edge that duplicates one the kept record already has.
// When the archived edge held the contact's primary-employer slot, the edge it
// duplicates takes the slot, so the contact keeps a current employer.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// slotHolder is the live edge holding the contact's primary-employer slot, or
// the zero UUID when none does.
func slotHolder(ctx context.Context, t *testing.T, e *dedupeEnv, contact ids.ContactID) ids.UUID {
	t.Helper()
	var edge ids.UUID
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM relationship r WHERE contact_id = $1 AND `+employment.CurrentPrimarySlotSQL("r"), contact).Scan(&edge)
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("reading the contact's primary slot: %v", err)
	}
	return edge
}

func TestACompanyMergeMovesThePrimarySlotToTheKeptDuplicate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Ada Pellinger", "Norrvik Old", "Norrvik New")
	e.employAt(ctx, t, fx.contact, fx.first, true)
	kept := e.employAt(ctx, t, fx.contact, fx.second, false)

	if _, err := e.store.MergeCompany(ctx, fx.first, fx.second, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if got := slotHolder(ctx, t, e, fx.contact); got != kept.ID {
		t.Errorf("the primary slot is held by %v, want the kept edge %v at the kept company", got, kept.ID)
	}
}

func TestAContactMergeMovesThePrimarySlotToTheKeptDuplicate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Bo Starling", "Quell Ltd", "Unused Ltd")
	survivor := mustContact(t, e, "Bo Starling")
	kept := e.employAt(ctx, t, survivor, fx.first, false)
	e.employAt(ctx, t, fx.contact, fx.first, true)

	if _, err := e.store.MergeContact(ctx, fx.contact, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if got := slotHolder(ctx, t, e, survivor); got != kept.ID {
		t.Errorf("the survivor's primary slot is held by %v, want its edge %v at the shared employer", got, kept.ID)
	}
}

func TestAContactMergeKeepsTheSurvivorsOwnPrimary(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Cyra Hollis", "Shared Works", "Own Works")
	survivor := mustContact(t, e, "Cyra Hollis")
	own := e.employAt(ctx, t, survivor, fx.second, true)
	shared := e.employAt(ctx, t, survivor, fx.first, false)
	e.employAt(ctx, t, fx.contact, fx.first, true)

	if _, err := e.store.MergeContact(ctx, fx.contact, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if got := slotHolder(ctx, t, e, survivor); got != own.ID {
		t.Errorf("the survivor's primary slot is held by %v, want its own primary %v", got, own.ID)
	}
	if got := countAuditRowsHolding(ctx, t, e.store, "relationship", shared.ID, "is_current_primary"); got != 0 {
		t.Errorf("the survivor's shared-employer edge has %d flag changes, want none", got)
	}
}

func TestThePromotedHeirIsAudited(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Dag Ostrander", "Fenwick Old", "Fenwick New")
	e.employAt(ctx, t, fx.contact, fx.first, true)
	kept := e.employAt(ctx, t, fx.contact, fx.second, false)
	events := outboxRowsNaming(ctx, t, e.store, kept.ID)

	if _, err := e.store.MergeCompany(ctx, fx.first, fx.second, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if got := countAuditRowsHolding(ctx, t, e.store, "relationship", kept.ID, "is_current_primary"); got != 1 {
		t.Errorf("the heir has %d flag changes, want 1", got)
	}
	wantFlagFlipRecorded(ctx, t, e, kept.ID, false, events)
	var origin string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT evidence ->> $2 FROM audit_log
			WHERE entity_type = 'relationship' AND entity_id = $1 AND action = 'update' AND after ? 'is_current_primary'`,
			kept.ID, auditKeyOrigin).Scan(&origin)
	}); err != nil {
		t.Fatalf("reading the heir's audit evidence: %v", err)
	}
	if origin != originMerge {
		t.Errorf("the heir's audit origin = %q, want %q", origin, originMerge)
	}
}

func TestACompanyMergeFilesTwinsForAContactWhoseEmployerMoved(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	survivor := mustEmployer(t, e, "Halberd Robotics GmbH")
	retired := mustEmployer(t, e, "Halberd Robotik GmbH")
	onFile := mustContact(t, e, "Jorinde Valkenburg")
	employ(t, e, onFile, survivor)
	twin := mustContact(t, e, "Jorinde Valkenburgh")
	e.employAt(ctx, t, twin, retired, true)
	e.employAt(ctx, t, twin, survivor, false)
	if pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID); len(pairs) != 0 {
		t.Fatalf("got %d pairs before the merge, want 0", len(pairs))
	}

	if _, err := e.store.MergeCompany(ctx, retired, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID)
	if len(pairs) != 1 || pairs[0].OtherID != onFile.String() {
		t.Fatalf("got pairs %+v, want exactly the pair with the contact on file", pairs)
	}
}

// monthStart is the first day of the database's current month. An end on that
// day is current at month precision and over at day precision. Two duplicate
// edges can then differ only in whether they are current.
func monthStart(ctx context.Context, t *testing.T, e *dedupeEnv) time.Time {
	t.Helper()
	var day time.Time
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT date_trunc('month', current_date)::date`).Scan(&day)
	}); err != nil {
		t.Fatalf("reading the database's month: %v", err)
	}
	return day
}

// employEnding creates an employment that ends at monthStart with the given
// precision. Only a current employment can be created as primary.
func (e *dedupeEnv) employEnding(ctx context.Context, t *testing.T, contact ids.ContactID, company ids.CompanyID, primary bool, ends time.Time, precision string) relationshipRow {
	t.Helper()
	edge, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: "employment", ContactID: &contact, CompanyID: &company,
		IsCurrentPrimary: &primary, EndedAt: &ends, EndedPrecision: &precision, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the employment: %v", err)
	}
	return edge
}

// lapsePrimary is a primary employment whose end date has passed. A patch that
// ends an employment drops its flag, so only time ends one that keeps it. This
// statement stands in for the month that went by.
func (e *dedupeEnv) lapsePrimary(ctx context.Context, t *testing.T, edge ids.UUID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE relationship SET ended_precision = 'day' WHERE id = $1`, edge)
		return err
	}); err != nil {
		t.Fatalf("lapse the primary employment: %v", err)
	}
}

func TestACompanyMergeHandsTheSlotOnlyBetweenCurrentEmployments(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	ends := monthStart(ctx, t, e)

	current := e.seedFlipFixture(ctx, t, "Eda Marsh", "Marsh Old", "Marsh New")
	e.employEnding(ctx, t, current.contact, current.first, true, ends, "month")
	endedHeir := e.employEnding(ctx, t, current.contact, current.second, false, ends, "day")

	ended := e.seedFlipFixture(ctx, t, "Finn Aske", "Aske Old", "Aske New")
	holder := e.employEnding(ctx, t, ended.contact, ended.first, true, ends, "month")
	currentHeir := e.employEnding(ctx, t, ended.contact, ended.second, false, ends, "month")
	e.lapsePrimary(ctx, t, holder.ID)

	for _, fx := range []flipFixture{current, ended} {
		if _, err := e.store.MergeCompany(ctx, fx.first, fx.second, nil); err != nil {
			t.Fatalf("merge: %v", err)
		}
	}

	if got := slotHolder(ctx, t, e, current.contact); got == endedHeir.ID {
		t.Error("an ended employment took the slot from a current one")
	}
	if got := slotHolder(ctx, t, e, ended.contact); got == currentHeir.ID {
		t.Error("an ended employment's slot went to a current one a user left non-primary")
	}
}

func TestAContactMergeHandsTheSlotOnlyBetweenCurrentEmployments(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	ends := monthStart(ctx, t, e)

	current := e.seedFlipFixture(ctx, t, "Gry Tollan", "Tollan Works", "Unused Works")
	currentSurvivor := mustContact(t, e, "Gry Tollan")
	endedHeir := e.employEnding(ctx, t, currentSurvivor, current.first, false, ends, "day")
	e.employEnding(ctx, t, current.contact, current.first, true, ends, "month")

	ended := e.seedFlipFixture(ctx, t, "Hal Brenner", "Brenner Works", "Unused Works")
	endedSurvivor := mustContact(t, e, "Hal Brenner")
	currentHeir := e.employEnding(ctx, t, endedSurvivor, ended.first, false, ends, "month")
	holder := e.employEnding(ctx, t, ended.contact, ended.first, true, ends, "month")
	e.lapsePrimary(ctx, t, holder.ID)

	if _, err := e.store.MergeContact(ctx, current.contact, currentSurvivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if _, err := e.store.MergeContact(ctx, ended.contact, endedSurvivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if got := slotHolder(ctx, t, e, currentSurvivor); got == endedHeir.ID {
		t.Error("an ended employment took the slot from a current one")
	}
	if got := slotHolder(ctx, t, e, endedSurvivor); got == currentHeir.ID {
		t.Error("an ended employment's slot went to a current one a user left non-primary")
	}
}
