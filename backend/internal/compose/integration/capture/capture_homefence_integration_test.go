// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Every member's cached Home is served while home_read_fence holds still. A
// mail's first link moves the fence when another transaction saw it unlinked.
// So capture files the mail under its sender before it commits.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestACapturedMailLeavesTheHomeFenceWhereItWas(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"a stranger's first mail", func(t *testing.T) {
			env.sync(t, email("fence-buyer@acme.example", "Fence Buyer", captureOwner, "fence-m1@acme.example", ""))
		}},
		{"the owner's reply, which mints the contact", func(t *testing.T) {
			env.syncSent(t, map[string]bool{"fence-m2@myco.example": true},
				email(captureOwner, "", "fence-buyer@acme.example", "fence-m2@myco.example", "fence-m1@acme.example"))
		}},
		{"the contact's next mail", func(t *testing.T) {
			env.sync(t, email("fence-buyer@acme.example", "Fence Buyer", captureOwner, "fence-m3@acme.example", "fence-m2@myco.example"))
		}},
	}
	for _, step := range steps {
		before := captureHomeFence(t, e)
		step.run(t)
		if moved := captureHomeFence(t, e) - before; moved != 0 {
			t.Errorf("%s moved the Home fence by %d, want 0: every member's cached Home ended", step.name, moved)
		}
	}
	// The fence held because the links were written inside the capture, not
	// because none were written.
	if n := countRows(t, e, `
		SELECT count(*) FROM activity_link al JOIN contact_email ce ON ce.contact_id = al.contact_id
		 WHERE ce.email = 'fence-buyer@acme.example'`); n != 2 {
		t.Fatalf("%d mails filed under the buyer, want 2 (the reply and the mail after it)", n)
	}

	before := captureHomeFence(t, e)
	archiveNewestMail(t, e)
	if moved := captureHomeFence(t, e) - before; moved != 1 {
		t.Errorf("archiving a captured mail moved the Home fence by %d, want 1: it narrows reach", moved)
	}
}

func captureHomeFence(t *testing.T, e *integration.SearchEnv) int {
	t.Helper()
	return countRows(t, e, `SELECT generation::int FROM home_read_fence`)
}

// archiveNewestMail archives the newest captured mail through the activities
// store, as the member's own archive does.
func archiveNewestMail(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true, Delete: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	var id ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM activity WHERE kind = 'email' AND archived_at IS NULL
			ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&id)
	}); err != nil {
		t.Fatalf("finding the newest mail: %v", err)
	}
	if _, err := activities.NewStore(e.DB()).ArchiveActivity(ctx, ids.From[ids.ActivityKind](id), nil); err != nil {
		t.Fatalf("archiving the mail: %v", err)
	}
}
