// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Querier is the read surface this file needs, satisfied by *pgx.Conn,
// *pgxpool.Pool and pgx.Tx alike — the three things a suite has in hand when it
// wants to name the installation's workspace.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// InstallationWorkspaceID resolves THE installation's workspace and refuses
// anything else.
//
// It exists because ADR-0091 removed the column suites used to select that row
// by. What replaced it must not merely return A row: `ORDER BY created_at LIMIT
// 1` answers even when the fixture is wrong, and after phase D there is no
// tenant column left on core for the mismatch to show up on later. A lookup
// that cannot fail is a fixture that cannot report being misbuilt.
//
// So it mirrors identity.activeWorkspaces, which is the production authority on
// this question (installation.go): archived rows are excluded, and more than
// one live row is refused rather than picked between. A suite that deliberately
// seeds a second workspace holds its id already — it minted it — and must use
// that, not this.
//
// The ordering carries `id` as a tiebreak because created_at is
// transaction_timestamp(): rows born in one statement share it exactly, so
// created_at alone leaves the winner to the planner.
func InstallationWorkspaceID(ctx context.Context, t *testing.T, q Querier) string {
	t.Helper()
	rows, err := q.Query(ctx, `
		SELECT id FROM workspace
		 WHERE archived_at IS NULL
		 ORDER BY created_at, id
		 LIMIT 2`)
	if err != nil {
		t.Fatalf("resolving the installation's workspace: %v", err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("resolving the installation's workspace: %v", err)
		}
		found = append(found, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("resolving the installation's workspace: %v", err)
	}
	switch len(found) {
	case 1:
		return found[0]
	case 0:
		t.Fatal("no live workspace exists — the fixture did not bootstrap, and every assertion keyed on this id would be about a row that is not there")
	default:
		t.Fatal("more than one live workspace exists, so there is no such thing as THE installation's workspace here — " +
			"a suite that seeded a second one already holds its id and must use that")
	}
	return ""
}

// InstallationWorkspaceUUID is InstallationWorkspaceID for the callers that
// hold the id as a value rather than a string. Same resolution, same refusals.
func InstallationWorkspaceUUID(ctx context.Context, t *testing.T, q Querier) ids.UUID {
	t.Helper()
	id, err := ids.Parse(InstallationWorkspaceID(ctx, t, q))
	if err != nil {
		t.Fatalf("the installation's workspace id is not a uuid: %v", err)
	}
	return id
}
