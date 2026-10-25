// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *promiseEnv) asEditor() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Update: true},
				"contact":  {Read: true, Update: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

func (e *promiseEnv) seedLinkedNote(t *testing.T, contactOwner ids.UUID) (activityID ids.UUID) {
	t.Helper()
	contactID := ids.NewV7()
	activityID = ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Linked Contact', $2, 'seed', 'system')`, contactID, contactOwner)
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'A note', now(), 'seed', 'system')`, activityID)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), activityID, contactID)
	return activityID
}

// The flag on the single read and on the list is the update path's answer.
// A rep with own scope is told a colleague's activity is read-only.
func TestActivityReadsSayWhetherTheCallerMayChangeIt(t *testing.T) {
	e := setupPromises(t)
	mine := e.seedLinkedNote(t, e.rep)
	theirs := e.seedLinkedNote(t, e.other)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))

	for id, want := range map[ids.UUID]bool{mine: true, theirs: false} {
		got, err := store.GetActivity(e.asEditor(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("reading activity %s: %v", id, err)
		}
		if got.Writable == nil || *got.Writable != want {
			t.Errorf("single read of %s: writable = %v, want %v", id, got.Writable, want)
		}
	}

	listed, _, err := store.ListActivities(e.asEditor(), ListActivitiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("list returned %d activities, want 2", len(listed))
	}
	for _, a := range listed {
		want := ids.UUID(a.Id) == mine
		if a.Writable == nil || *a.Writable != want {
			t.Errorf("list row %s: writable = %v, want %v", a.Id, a.Writable, want)
		}
	}

	if err := store.tx(e.asEditor(), func(tx pgx.Tx) error {
		return auth.EnsureActivityWritable(e.asEditor(), tx, theirs)
	}); err == nil {
		t.Error("the write gate admitted the activity the read called read-only")
	}
}

// A seat without the activity:update grant is told nothing is writable, even
// on an activity it authored.
func TestActivityReadsAreReadOnlyWithoutTheUpdateGrant(t *testing.T) {
	e := setupPromises(t)
	mine := e.seedLinkedNote(t, e.rep)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	got, err := store.GetActivity(e.as(), ids.From[ids.ActivityKind](mine), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Writable == nil || *got.Writable {
		t.Errorf("writable = %v for a read-only grant, want false", got.Writable)
	}
}
