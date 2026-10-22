// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A write to one employment can move the current-employer flag on another edge
// of the same contact. That edge gets its own audit row and its own event.
// Without them its history cannot say when it became or stopped being the
// contact's current employer.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// flipFixture is one contact and two employers, neither employment written yet.
type flipFixture struct {
	contact       ids.ContactID
	first, second ids.CompanyID
}

func (e *dedupeEnv) seedFlipFixture(ctx context.Context, t *testing.T, name, first, second string) flipFixture {
	t.Helper()
	var fx flipFixture
	for i, companyName := range []string{first, second} {
		company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: companyName, Source: "manual"})
		if err != nil {
			t.Fatalf("seed %s: %v", companyName, err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
		if i == 0 {
			fx.first = id
		} else {
			fx.second = id
		}
	}
	contact, err := e.store.CreateContact(ctx, CreateContactInput{FullName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("seed contact %s: %v", name, err)
	}
	fx.contact = ids.From[ids.ContactKind](ids.UUID(contact.Id))
	return fx
}

func (e *dedupeEnv) employAt(ctx context.Context, t *testing.T, contact ids.ContactID, company ids.CompanyID, primary bool) relationshipRow {
	t.Helper()
	edge, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: "employment", ContactID: &contact, CompanyID: &company,
		IsCurrentPrimary: &primary, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the employment: %v", err)
	}
	if edge.IsCurrentPrimary != primary {
		t.Fatalf("the new employment's flag = %v, want %v", edge.IsCurrentPrimary, primary)
	}
	return edge
}

// outboxRowsNaming counts the events whose envelope names the edge. The outbox
// has no audit_id column, so the edge id in the envelope is the link.
func outboxRowsNaming(ctx context.Context, t *testing.T, s *Store, edge ids.UUID) int {
	t.Helper()
	var n int
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1::text || '%'`, edge).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the edge's events: %v", err)
	}
	return n
}

// wantFlagFlipRecorded checks the edge's latest flag audit row, and that one
// more event names it than before the write.
func wantFlagFlipRecorded(ctx context.Context, t *testing.T, e *dedupeEnv, edge ids.UUID, was bool, eventsBefore int) {
	t.Helper()
	before, after := auditImagesHolding(ctx, t, e.store, "relationship", edge, "is_current_primary")
	wantImage(t, before, "before", "is_current_primary", was)
	wantImage(t, after, "after", "is_current_primary", !was)
	if got := outboxRowsNaming(ctx, t, e.store, edge); got != eventsBefore+1 {
		t.Errorf("%d events name the edge, want %d: no consumer hears its flag moved", got, eventsBefore+1)
	}
}

func TestCreatingANewPrimaryEmploymentRecordsTheDemotedOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	fx := e.seedFlipFixture(ctx, t, "Ilka Sandvik", "Nordhaven AG", "Brightmoor Ltd")
	old := e.employAt(ctx, t, fx.contact, fx.first, true)
	events := outboxRowsNaming(ctx, t, e.store, old.ID)

	e.employAt(ctx, t, fx.contact, fx.second, true)

	wantFlagFlipRecorded(ctx, t, e, old.ID, true, events)
}

func TestPatchingAnEmploymentToPrimaryRecordsTheDemotedOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.asEdgeEditor(t)
	fx := e.seedFlipFixture(ctx, t, "Tomas Reiter", "Quellwerk GmbH", "Salvia Partners")
	old := e.employAt(ctx, t, fx.contact, fx.first, true)
	other := e.employAt(ctx, t, fx.contact, fx.second, false)
	events := outboxRowsNaming(ctx, t, e.store, old.ID)

	primary := true
	if _, err := e.store.UpdateRelationship(ctx, other.ID, UpdateRelationshipInput{IsCurrentPrimary: &primary}); err != nil {
		t.Fatalf("mark the second employment primary: %v", err)
	}

	wantFlagFlipRecorded(ctx, t, e, old.ID, true, events)
}

func TestEndingThePrimaryEmploymentRecordsThePromotedOne(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.asEdgeEditor(t)
	fx := e.seedFlipFixture(ctx, t, "Noor Akhtar", "Fennick Labs", "Oakridge Bank")
	old := e.employAt(ctx, t, fx.contact, fx.first, true)
	survivor := e.employAt(ctx, t, fx.contact, fx.second, false)
	events := outboxRowsNaming(ctx, t, e.store, survivor.ID)

	left := time.Date(2025, time.January, 31, 0, 0, 0, 0, time.UTC)
	if _, err := e.store.UpdateRelationship(ctx, old.ID, UpdateRelationshipInput{EndedAt: &left}); err != nil {
		t.Fatalf("end the primary employment: %v", err)
	}

	wantFlagFlipRecorded(ctx, t, e, survivor.ID, false, events)
}
