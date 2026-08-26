// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The retention sweep's activity/erase destroys the message, not one copy of it.
//
// Clearing activity.body while raw_capture kept the verbatim provider payload
// erased nothing: the two are joined on (source_system, source_id), which the
// erase deliberately preserves so the record of the message survives, and
// privacy/sar.go exports raw_capture by email match. An Art. 15 package
// therefore handed back the full original of a message whose retention window
// had closed years earlier.
//
// raw_capture has no sweep of its own and cannot grow one usefully: the only
// other purge is Art. 17 erasure, which is scoped to a PERSON, and a retention
// window is scoped to time. So the sweep is the only thing that can age it out.
//
// Driven through compose.NewRetentionServiceFor rather than a service this test
// wires itself, because the defect was a MISSING seam — a test that supplies
// its own wiring proves nothing about the one the worker runs.

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/modules/privacy"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func TestTheRetentionSweepDestroysTheProviderOriginalToo(t *testing.T) {
	e := Setup(t)
	activity := ids.NewV7()
	const sourceSystem, sourceID = "gmail", "msg-aged-out"

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('activity', NULL, 100, 'erase')`); err != nil {
			return err
		}
		// The activity and its original, joined the way capture writes them:
		// storeRawCapture keys on the message's natural key, and the activity
		// carries the same pair.
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, source_system, source_id)
			VALUES ($1, 'email', 'Quarterly figures', 'the numbers themselves',
			        now() - interval '400 days', 'capture', 'connector:t', $2, $3)`,
			activity, sourceSystem, sourceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO raw_capture (source_system, source_id, payload)
			VALUES ($1, $2, $3)`,
			sourceSystem, sourceID, []byte(`{"subject":"Quarterly figures","body":"the numbers themselves"}`)); err != nil {
			return err
		}
		// Provenance for the body about to be erased. Both sibling erasers
		// delete these; the sweep did not, so the row went on naming who
		// captured the erased text and from where.
		_, err := tx.Exec(ctx, `
			INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by)
			VALUES ('activity', $1, 'body', 'capture', 'connector:t')`, activity)
		return err
	}); err != nil {
		t.Fatalf("seeding the aged-out message: %v", err)
	}

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}

	var body *string
	var originals, provenance int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `SELECT body FROM activity WHERE id = $1`, activity).Scan(&body); err != nil {
			return err
		}
		// The join the erasure leaves standing, which is the query an Art. 15
		// export runs: the record still names its provider message, and that is
		// deliberate — what must be gone is what the message SAID.
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM raw_capture r
			  JOIN activity a ON r.source_system = a.source_system AND r.source_id = a.source_id
			 WHERE a.id = $1`, activity).Scan(&originals); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM field_provenance
			 WHERE object_type = 'activity' AND object_id = $1`, activity).Scan(&provenance)
	}); err != nil {
		t.Fatalf("reading the record after the sweep: %v", err)
	}

	if body != nil {
		t.Errorf("activity.body = %q after an erase policy fired, want cleared", *body)
	}
	if originals != 0 {
		t.Errorf("%d raw_capture original(s) survived the erasure of the activity they duplicate — "+
			"the content is one join away, and SAR exports raw_capture by email match", originals)
	}
	if provenance != 0 {
		t.Errorf("%d field_provenance row(s) survived, still naming who captured the erased body "+
			"and from where; both sibling erasers delete them", provenance)
	}
}

// TestTheEraseActionRefusesWithoutItsPurger holds the half of the fix a passing
// sweep cannot show: that an unwired purger FAILS rather than quietly erasing
// the copy and keeping the original. Without this, deleting the seam from
// compose would leave every assertion above passing for a service nobody builds.
func TestTheEraseActionRefusesWithoutItsPurger(t *testing.T) {
	e := Setup(t)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('activity', NULL, 100, 'erase')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity (kind, subject, body, occurred_at, source, captured_by)
			VALUES ('email', 'Quarterly figures', 'the numbers themselves',
			        now() - interval '400 days', 'capture', 'connector:t')`)
		return err
	}); err != nil {
		t.Fatalf("seeding the aged-out message: %v", err)
	}

	// The unassembled service — what privacy.NewRetentionService builds on its
	// own, which is the configuration compose must never ship.
	bare := privacy.NewRetentionService(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := bare.EvaluateInstallation(RetentionPassCtx(e.WS)); err == nil {
		t.Fatal("an erase policy ran on a service with no raw-capture purger and reported success; " +
			"it must refuse, because the provider original has no other way to age out")
	}
}
