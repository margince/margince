// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A qualification in flight when an erasure starts WINS, and its correspondence
// is held rather than destroyed.
//
// The erasure decides erase-or-hold by reading `activity.retention_class` — the
// column a deal win or a sent offer stamps, in its own transaction. Both read
// and write the same rows, and the erasure used to lock only the subject's
// identifier keys: not the activities, and not the deals they hang off. So the
// two could interleave with the erasure reading a NULL class, destroying the
// correspondence, and the qualification committing afterwards.
//
// Nothing readable survived that either way — the row is gone — but A165 /
// ADR-0114 §1 says the correspondence was to be HELD, and the obligation was
// not honoured (issue #1618). The class is monotonic, so the failure is not
// symmetric: a hold that should have been an erase cannot happen, and an erase
// that should have been a hold destroys evidence somebody is required to keep.
//
// The lock is what gives the race a winner. This test makes the interleaving
// happen rather than waiting for it: the stamp's rows are pinned by a
// transaction of this test's own, the erasure is started and must BLOCK, the
// stamp lands, and the erasure then proceeds — and must hold.

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAQualificationInFlightHoldsItsCorrespondence(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	// A deal the correspondence hangs off, so the stamp below is the one the
	// product runs — StampCorrespondenceForDeal works from activity_link.
	deal := linkCorrespondenceToADeal(t, e, f.email)

	// The Handelsbrief carries NO retention class yet — this is a lead nobody
	// has qualified. Unstamped on purpose: a row already stamped is held by the
	// ordinary path and proves nothing about the race.
	assertRetentionClass(t, e, f.email, "")

	// The qualification, held open in its own transaction. It IS the holder —
	// which is the real interleaving, and simpler than pinning the row with a
	// third party: the erasure must wait for the stamp itself.
	holderCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	holder, err := e.Pool.Begin(holderCtx)
	if err != nil {
		t.Fatal(err)
	}
	// The REAL stamp, not a hand-written UPDATE. The class and its evidence are
	// one statement on purpose — a CHECK refuses a restricted row whose
	// evidence does not say what qualified it — and a test that wrote only the
	// column would be racing something the product never does.
	var holderPID int
	if err := holder.QueryRow(holderCtx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	if err := activities.StampCorrespondenceForDeal(holderCtx, holder, deal, "deal_won"); err != nil {
		t.Fatalf("stamping the qualifying deal's correspondence: %v", err)
	}

	erased := make(chan error, 1)
	go func() {
		erased <- privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), f.person, "test")
	}()

	// Wait until the erasure is genuinely PARKED behind the stamp, asked of
	// Postgres rather than assumed after a sleep. Without this the stamp could
	// commit before the erasure has opened its transaction, and the test would
	// pass against an erasure that takes no lock at all.
	waitForBlockedOn(holderCtx, t, holder, holderPID)

	// The qualification commits while the erasure waits, so the class the
	// erasure eventually reads is one that did not exist when it started.
	if err := holder.Commit(holderCtx); err != nil {
		t.Fatalf("committing the qualification: %v", err)
	}

	if err := <-erased; err != nil {
		t.Fatalf("erasing the subject → %v", err)
	}

	// The whole assertion: the erasure saw the class the qualification wrote,
	// and held the correspondence rather than destroying it.
	assertHeldNotDestroyed(t, e, f.email)
}

func assertRetentionClass(t *testing.T, e *Env, activity ids.UUID, want string) {
	t.Helper()
	var class string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(retention_class, '') FROM activity WHERE id = $1`, activity).Scan(&class)
	}); err != nil {
		t.Fatalf("reading the retention class: %v", err)
	}
	if class != want {
		t.Fatalf("retention_class = %q, want %q", class, want)
	}
}

func assertHeldNotDestroyed(t *testing.T, e *Env, activity ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var subject, class string
		var body *string
		var restricted bool
		if err := tx.QueryRow(context.Background(), `
			SELECT subject, body, restricted_at IS NOT NULL, coalesce(retention_class, '')
			  FROM activity WHERE id = $1`, activity).Scan(&subject, &body, &restricted, &class); err != nil {
			return err
		}
		if class != "commercial_correspondence" {
			return fmt.Errorf("retention_class = %q — the qualification's stamp did not survive", class)
		}
		// The substance is what the obligation is about. An erased row has an
		// emptied body and the tombstone name; a held one keeps both.
		if body == nil {
			return fmt.Errorf("the correspondence was DESTROYED: the erasure read a null class "+
				"and ran its destroy arm before the qualification committed (subject=%q)", subject)
		}
		if !restricted {
			return fmt.Errorf("the correspondence survived but was not restricted: subject=%q", subject)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// linkCorrespondenceToADeal gives the Handelsbrief a deal to be qualified BY,
// which is what StampCorrespondenceForDeal reads: it stamps every activity
// linked to the deal, so the link is the whole fixture this race needs.
func linkCorrespondenceToADeal(t *testing.T, e *Env, activity ids.UUID) ids.DealID {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	var deal ids.DealID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		id := ids.NewV7()
		if _, err := tx.Exec(ctx, `
			INSERT INTO deal (id, name, pipeline_id, stage_id, status, source, captured_by)
			VALUES ($1, 'Qualifying deal', $2, $3, 'open', 'manual', 'human:x')`,
			id, pipeline, open); err != nil {
			return err
		}
		deal = ids.From[ids.DealKind](id)
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, deal_id)
			VALUES ($1, 'deal', $2)`, activity, id)
		return err
	}); err != nil {
		t.Fatalf("seeding the qualifying deal: %v", err)
	}
	return deal
}
