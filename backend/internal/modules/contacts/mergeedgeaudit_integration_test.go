// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A merge moves or archives the merged-away record's edges, and each edge gets
// its own audit row and event. Without them an edge's history cannot say that
// it moved to another record or lost its current-employer flag on the way.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// edgeAuditRows counts the audit rows of one action on one edge.
func edgeAuditRows(ctx context.Context, t *testing.T, s *Store, edge ids.UUID, action string) int {
	t.Helper()
	var n int
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log
			WHERE entity_type = 'relationship' AND entity_id = $1 AND action = $2`, edge, action).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the edge's %s rows: %v", action, err)
	}
	return n
}

func TestAContactMergeRecordsEachEdgeItMovesOrArchives(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	fx := e.seedFlipFixture(ctx, t, "Vera Lindqvist", "Halden Works", "Corvane Group")
	survivor, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Vera Lindqvist", Source: "manual"})
	if err != nil {
		t.Fatalf("seed the survivor: %v", err)
	}
	target := ids.From[ids.ContactKind](ids.UUID(survivor.Id))
	moved := e.employAt(ctx, t, fx.contact, fx.first, true)
	duplicate := e.employAt(ctx, t, fx.contact, fx.second, false)
	e.employAt(ctx, t, target, fx.second, true)
	movedEvents := outboxRowsNaming(ctx, t, e.store, moved.ID)
	duplicateEvents := outboxRowsNaming(ctx, t, e.store, duplicate.ID)

	if _, err := e.store.MergeContact(ctx, fx.contact, target, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	before, after := auditImagesHolding(ctx, t, e.store, "relationship", moved.ID, endContactColumn)
	wantImage(t, before, "before", endContactColumn, fx.contact.String())
	wantImage(t, after, "after", endContactColumn, target.String())
	wantImage(t, before, "before", "is_current_primary", true)
	wantImage(t, after, "after", "is_current_primary", false)
	if got := outboxRowsNaming(ctx, t, e.store, moved.ID); got != movedEvents+1 {
		t.Errorf("%d events name the moved edge, want %d", got, movedEvents+1)
	}
	if got := edgeAuditRows(ctx, t, e.store, duplicate.ID, "archive"); got != 1 {
		t.Errorf("the duplicate edge the merge archived has %d archive rows, want 1", got)
	}
	if got := outboxRowsNaming(ctx, t, e.store, duplicate.ID); got != duplicateEvents+1 {
		t.Errorf("%d events name the archived duplicate, want %d", got, duplicateEvents+1)
	}
}

func TestACompanyMergeRecordsEachEdgeItMoves(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Ilse Drummond", "Pellow Old", "Pellow New")
	edge := e.employAt(ctx, t, fx.contact, fx.first, true)
	events := outboxRowsNaming(ctx, t, e.store, edge.ID)

	if _, err := e.store.MergeCompany(ctx, fx.first, fx.second, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	before, after := auditImagesHolding(ctx, t, e.store, "relationship", edge.ID, endCompanyColumn)
	wantImage(t, before, "before", endCompanyColumn, fx.first.String())
	wantImage(t, after, "after", endCompanyColumn, fx.second.String())
	if _, moved := after["is_current_primary"]; moved {
		t.Errorf("the flag did not move and the row says it did: %v", after)
	}
	if got := outboxRowsNaming(ctx, t, e.store, edge.ID); got != events+1 {
		t.Errorf("%d events name the moved edge, want %d", got, events+1)
	}
}
