// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNoticeDutyCanMoveFromAFormerOfficerWithoutRestartingItsDeadline(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	former := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO app_user(id,email,display_name) VALUES(@id,'former@example.test','Former officer')`, pgx.StrictNamedArgs{"id": former}); err != nil {
		t.Fatal(err)
	}
	id := seedNoticeCase(t, e, "purchased_or_imported", []string{"record_confirmation"})
	before, err := e.store.AssignNoticeCase(ctx, id, former)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE app_user SET status='deactivated' WHERE id=@id`, pgx.StrictNamedArgs{"id": former}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.AssignNoticeCase(ctx, id, former); !errors.As(err, new(*ValidationError)) {
		t.Fatalf("routing to a former officer: %v", err)
	}
	after, err := e.store.AssignNoticeCase(ctx, id, e.user)
	if err != nil {
		t.Fatal(err)
	}
	if after.OwnerUserID == nil || *after.OwnerUserID != e.user || after.State != before.State || !reflect.DeepEqual(after.DueAt, before.DueAt) {
		t.Fatalf("reassignment changed the duty: before=%+v after=%+v", before, after)
	}
}

func TestDSRReassignmentRefusesAFormerOfficerAndPreservesTheCase(t *testing.T) {
	e := setupDSR(t)
	former := ids.NewV7()
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO app_user(id,email,display_name) VALUES(@id,'former@example.test','Former officer')`, pgx.StrictNamedArgs{"id": former}); err != nil {
		t.Fatal(err)
	}
	previous := ids.From[ids.UserKind](former)
	before, err := e.store.CreateDSR(e.ctx, CreateDSRInput{Kind: "access", SubjectRef: "email:subject@example.test", AssigneeID: &previous})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE app_user SET status='deactivated' WHERE id=@id`, pgx.StrictNamedArgs{"id": former}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.CreateDSR(e.ctx, CreateDSRInput{Kind: "access", SubjectRef: "email:other@example.test", AssigneeID: &previous}); !errors.As(err, new(*ValidationError)) {
		t.Fatalf("new case routed to a former officer: %v", err)
	}
	if _, err := e.store.UpdateDSR(e.ctx, before.ID, UpdateDSRInput{AssigneeID: &previous}); !errors.As(err, new(*ValidationError)) {
		t.Fatalf("case routed to a former officer: %v", err)
	}
	successor := ids.From[ids.UserKind](e.user)
	after, err := e.store.UpdateDSR(e.ctx, before.ID, UpdateDSRInput{AssigneeID: &successor})
	if err != nil {
		t.Fatal(err)
	}
	if after.AssigneeID == nil || *after.AssigneeID != successor || after.Status != before.Status || !after.DueAt.Equal(before.DueAt) {
		t.Fatalf("reassignment changed the case: before=%+v after=%+v", before, after)
	}
}
