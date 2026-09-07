// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The seeded Team Lead reads their team, and the seeded rep does not.
//
// The `team` arm of OwnerPredicate was written, reviewed and unreached for
// months: no seeded role used it, so nothing exercised the branch and nothing
// would have noticed it breaking. It is reached now — by `manager` — and this
// is what keeps that true.
//
// Read off the SEEDED ROW rather than the compiled defaults, because those are
// two different facts. seedSystemRoles writes each document once at workspace
// creation and never re-syncs, so a deployed installation carries whatever it
// was seeded with plus whatever a migration has since done to it; a test
// against the Go map would pass over a database that disagrees with it.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestTheSeededRolesReachTheRowScopeTheyClaim(t *testing.T) {
	svc, _, _ := seatChoosingALanguage(t)

	for _, want := range []struct {
		role, scope, why string
	}{
		{"manager", "team", "a Team Lead who reaches only their own rows is a name without reach: " +
			"they could not open a colleague's queue at all"},
		{"rep", "own", "team membership is not by itself permission to rewrite a teammate's records"},
		{"management", "all", "the unbounded grid, which is what makes it the one that is not team-bounded"},
	} {
		t.Run(want.role, func(t *testing.T) {
			var scope string
			if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(),
					`SELECT permissions ->> 'row_scope' FROM role WHERE key = $1 AND is_system`,
					want.role).Scan(&scope)
			}); err != nil {
				t.Fatalf("reading the seeded %s role: %v", want.role, err)
			}
			if scope != want.scope {
				t.Errorf("the seeded %s role carries row_scope %q, want %q — %s",
					want.role, scope, want.scope, want.why)
			}
		})
	}
}
