// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// OrganizationLinkedInReach answers "who on our team can get us in", and the
// answer has to be somebody who can actually be asked.
//
// The join filtered `u.archived_at IS NULL` and nothing else, which is only
// half of "still works here": deactivating an account sets `status` and leaves
// `archived_at` NULL, so a colleague who no longer works here kept being
// offered as a warm route into an account. The function had no test at all,
// which is how that survived from the day it landed.
//
// Both directions are asserted, and that is the point rather than thoroughness
// for its own sake: a test that only checks the deactivated seat is absent
// passes just as well against a query that returns nobody.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func TestReachCountsOnlyColleaguesWhoStillWorkHere(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	org := e.seedOrgNamed(t, "Acme GmbH")

	// One connection each, so a count that loses a seat is unambiguous: the
	// map either names them or it does not.
	e.seedReach(ctx, t, e.rep, org, "Abbas Fawaz")
	e.seedReach(ctx, t, e.otherRep, org, "Bea Hoffmann")

	both := e.reachInto(ctx, t, org)
	if both[e.rep] != 1 || both[e.otherRep] != 1 {
		t.Fatalf("with both seats live the reach is %v, want one connection each for %s and %s — "+
			"the rest of this test cannot mean anything if the baseline is already wrong",
			both, e.rep, e.otherRep)
	}

	e.deactivate(ctx, t, e.otherRep)

	after := e.reachInto(ctx, t, org)
	if _, offered := after[e.otherRep]; offered {
		t.Errorf("a deactivated colleague is still offered as a route into the account: %v", after)
	}
	if after[e.rep] != 1 {
		t.Errorf("the live colleague's reach is %v, want 1 — deactivating one seat took another seat's "+
			"count with it", after)
	}
}

// seedReach gives a colleague one LinkedIn connection already matched to an
// account. The import path is exercised by linkedin_integration_test.go; what
// this suite is about is which colleagues the count admits, so the connection
// arrives ready-matched.
//
// 'suggested' and not 'confirmed': matchGhostOrganizations attaches a ghost to
// an ACCOUNT by employer name even when the person never matches, and a
// 'confirmed' row needs a matched_person_id it would not have. The org-matched,
// person-unmatched row IS the reach case.
func (e *dedupeEnv) seedReach(ctx context.Context, t *testing.T, owner ids.UUID, org ids.OrganizationID, name string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO linkedin_connection
			  (owner_user_id, full_name, normalized_name, company_name,
			   normalized_company, matched_org_id, match_status, source)
			VALUES ($1, $2, lower($2), 'Acme GmbH', 'acme gmbh', $3, 'suggested', 'csv_export')`,
			owner, name, org)
		return err
	}); err != nil {
		t.Fatalf("seeding a connection for %s: %v", owner, err)
	}
}

// deactivate is what an admin does to a seat somebody has left: status moves
// and archived_at stays NULL, which is exactly the state the old predicate
// could not see.
func (e *dedupeEnv) deactivate(ctx context.Context, t *testing.T, user ids.UUID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, user)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("deactivating %s touched %d rows, want 1 — the seat this test is about was never changed",
				user, tag.RowsAffected())
		}
		return nil
	}); err != nil {
		t.Fatalf("deactivating %s: %v", user, err)
	}
	// The seat must still be here, or the test proves the archived half rather
	// than the status half — the two are indistinguishable from the outside.
	var archived *string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT archived_at::text FROM app_user WHERE id = $1`, user).Scan(&archived)
	}); err != nil {
		t.Fatalf("reading %s back: %v", user, err)
	}
	if archived != nil {
		t.Fatalf("deactivating %s also archived it (%s), so this test no longer distinguishes the two halves",
			user, *archived)
	}
}

func (e *dedupeEnv) reachInto(ctx context.Context, t *testing.T, org ids.OrganizationID) map[ids.UUID]int {
	t.Helper()
	var out map[ids.UUID]int
	if err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = OrganizationLinkedInReach(ctx, tx, org)
		return err
	}); err != nil {
		t.Fatalf("OrganizationLinkedInReach: %v", err)
	}
	return out
}
