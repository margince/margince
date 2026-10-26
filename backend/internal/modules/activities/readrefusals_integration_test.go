// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The transaction-held reads and the outside-id lookup answer only a caller who
// may read activities, and a missing activity is an error.
func TestActivityReadsInATransactionRefuseWhatTheyMust(t *testing.T) {
	e := setupPromises(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	noRead := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: e.rep})

	if _, err := store.ActivityIDBySource(noRead, "erp", "1"); err == nil {
		t.Error("ActivityIDBySource admitted a caller without activity:read")
	}
	tx, err := e.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var _ pgx.Tx = tx

	if _, err := store.GetActivityTx(noRead, tx, ids.New[ids.ActivityKind](), storekit.LiveOnly, CustomColumns{}); err == nil {
		t.Error("GetActivityTx admitted a caller without activity:read")
	}
	if _, err := store.GetActivityTx(e.asEditor(), tx, ids.New[ids.ActivityKind](), storekit.LiveOnly, CustomColumns{}); err == nil {
		t.Error("GetActivityTx of a missing activity succeeded")
	}
	if _, _, err := store.ListActivitiesIn(noRead, tx, ListActivitiesInput{}); err == nil {
		t.Error("ListActivitiesIn admitted a caller without activity:read")
	}
}
