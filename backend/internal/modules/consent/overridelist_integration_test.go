// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestStandingExceptionsArePagedAndCanBeRevoked(t *testing.T) {
	e := setupChannelConsent(t)
	wanted := make(map[ids.UUID]bool)
	for range 3 {
		id, err := e.store.Allow(e.ctx, AllowInput{
			ContactID: e.contact, Category: "requested_followup", Reason: "The contact asked for the follow-up.",
		})
		if err != nil {
			t.Fatal(err)
		}
		wanted[id] = true
	}
	first, page, err := e.store.ListContactOverrides(e.ctx, e.contact, 2, "")
	if err != nil || len(first) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("first page = %v, %+v, %v", first, page, err)
	}
	last, end, err := e.store.ListContactOverrides(e.ctx, e.contact, 2, page.NextCursor)
	if err != nil || len(last) != 1 || end.HasMore {
		t.Fatalf("last page = %v, %+v, %v", last, end, err)
	}
	for _, row := range append(first, last...) {
		id := ids.UUID(row.OverrideId)
		if !wanted[id] || !row.MayRevoke || row.Category != "requested_followup" {
			t.Fatalf("unexpected standing exception: %+v", row)
		}
		delete(wanted, id)
	}
	if len(wanted) != 0 {
		t.Fatalf("the pages missed %v", wanted)
	}
	removed := ids.UUID(first[0].OverrideId)
	if err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: removed, Reason: "The recorded context was wrong.",
	}); err != nil {
		t.Fatal(err)
	}
	remaining, _, err := e.store.ListContactOverrides(e.ctx, e.contact, 50, "")
	if err != nil || len(remaining) != 2 {
		t.Fatalf("after revocation = %v, %v", remaining, err)
	}
	for _, row := range remaining {
		if ids.UUID(row.OverrideId) == removed {
			t.Fatal("the revoked exception is still listed")
		}
	}
}

func TestStandingExceptionReadUsesContactAccessAndRevocationAuthority(t *testing.T) {
	e := setupChannelConsent(t)
	id, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "requested_followup", Reason: "They asked for the next steps.",
	})
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the fixture has no actor")
	}
	actor.Permissions = principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"contact": {Read: true}},
	}
	reader := principal.WithActor(e.ctx, actor)
	rows, _, err := e.store.ListContactOverrides(reader, e.contact, 50, "")
	if err != nil || len(rows) != 1 || rows[0].MayRevoke {
		t.Fatalf("read-only exception list = %v, %v", rows, err)
	}
	if err := e.store.RevokeOverride(reader, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: id, Reason: "Not my grant.",
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read-only revocation = %v", err)
	}
	if _, _, err := e.store.ListContactOverrides(reader, e.contact, 50, "bad cursor"); err == nil {
		t.Fatal("an invalid cursor was accepted")
	}
	other := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other rep')`, other, other.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE contact SET visibility = 'owner', owner_id = $1 WHERE id = $2`, other, e.contact); err != nil {
		t.Fatal(err)
	}
	if rows, _, err := e.store.ListContactOverrides(reader, e.contact, 50, ""); !errors.Is(err, apperrors.ErrNotFound) || len(rows) != 0 {
		t.Fatalf("private exceptions disclosed = %v, %v", rows, err)
	}
}

func TestCarriedStandingExceptionKeepsItsOriginalAuthorAndCompany(t *testing.T) {
	e := setupChannelConsent(t)
	recorder := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO app_user (id, email, display_name)
		VALUES ($1, $2, 'Original Rep')`, recorder, "original-"+recorder.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	source := seedOverrideContact(t, e, "Original contact")
	if _, err := e.owner.Exec(context.Background(), `UPDATE contact SET owner_id = $1 WHERE id = $2`, recorder, source); err != nil {
		t.Fatal(err)
	}
	company := overrideTestCompany(t, e, recorder, "Original Company")
	if _, err := e.store.Allow(boundedRepCtx(e.ws, recorder), AllowInput{
		ContactID: source, Category: "requested_followup", Reason: "Asked for a follow-up.",
	}); err != nil {
		t.Fatal(err)
	}
	before, _, err := e.store.ListContactOverrides(e.ctx, source, 50, "")
	if err != nil || len(before) != 1 {
		t.Fatalf("original exception = %v, %v", before, err)
	}
	overrideTestCompany(t, e, e.user, "Merger Company")
	survivor := seedOverrideContact(t, e, "Surviving contact")
	carryOverrides(t, e, source, survivor)
	after, _, err := e.store.ListContactOverrides(e.ctx, survivor, 50, "")
	if err != nil || len(after) != 1 {
		t.Fatalf("carried exception = %v, %v", after, err)
	}
	row := after[0]
	if row.SendingEntityId == nil || ids.UUID(*row.SendingEntityId) != company ||
		row.SendingEntityName == nil || *row.SendingEntityName != "Original Company" ||
		row.RecordedByName == nil || *row.RecordedByName != "Original Rep" ||
		!row.RecordedAt.Equal(before[0].RecordedAt) {
		t.Fatalf("the carry changed provenance or company: %+v", row)
	}
}

func TestAStandingExceptionDoesNotAcquireTheReadersNewCompany(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.store.Allow(e.ctx, AllowInput{ContactID: e.contact, Category: "requested_followup", Reason: "Requested next steps."}); err != nil {
		t.Fatal(err)
	}
	before, _, err := e.store.ListContactOverrides(e.ctx, e.contact, 50, "")
	if err != nil || len(before) != 1 || before[0].SendingEntityId == nil {
		t.Fatalf("original exception = %v, %v", before, err)
	}
	current := overrideTestCompany(t, e, e.user, "Current Company")
	rows, _, err := e.store.ListContactOverrides(e.ctx, e.contact, 50, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("exceptions = %v, %v", rows, err)
	}
	if rows[0].SendingEntityId == nil || *rows[0].SendingEntityId != *before[0].SendingEntityId || ids.UUID(*rows[0].SendingEntityId) == current {
		t.Fatalf("the exception acquired the reader's new company: %+v", rows[0])
	}
}

func overrideTestCompany(t *testing.T, e *channelConsentEnv, user ids.UUID, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO legal_entity
		(id, display_name, legal_name, country, imprint, captured_by)
		VALUES ($1, $2, $2, 'de', 'Company details', 'human:x')`, id, name); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO legal_entity_member
		(legal_entity_id, user_id, assigned_by) VALUES ($1, $2, 'human:x')
        ON CONFLICT (user_id) DO UPDATE SET legal_entity_id = EXCLUDED.legal_entity_id`, id, user); err != nil {
		t.Fatal(err)
	}
	return id
}
