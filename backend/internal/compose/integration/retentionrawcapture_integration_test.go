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
	"errors"
	"io"
	"log/slog"
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
		// Provenance for the body about to be erased: without the delete it
		// goes on naming who captured the text and from where, and it is
		// SAR-exported.
		_, err := tx.Exec(ctx, `
			INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by)
			VALUES ('activity', $1, 'body', 'capture', 'connector:t')`, activity)
		return err
	}); err != nil {
		t.Fatalf("seeding the aged-out message: %v", err)
	}

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}

	var body *string
	var keptKey bool
	var originals, provenance int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			SELECT body, source_system IS NOT DISTINCT FROM $2 AND source_id IS NOT DISTINCT FROM $3
			  FROM activity WHERE id = $1`,
			activity, sourceSystem, sourceID).Scan(&body, &keptKey); err != nil {
			return err
		}
		// Counted by the seeded key rather than through the join to activity.
		// The join reads zero for two different reasons — the original is gone,
		// or the erase stopped keeping the pair — and only one of those is this
		// test passing. Which pair the activity still carries is asserted
		// separately, above.
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM raw_capture WHERE source_system = $1 AND source_id = $2`,
			sourceSystem, sourceID).Scan(&originals); err != nil {
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
	if !keptKey {
		t.Error("the erase dropped the activity's (source_system, source_id) — the record of the message " +
			"is supposed to survive its content, and the purge below is keyed on that pair")
	}
	if originals != 0 {
		t.Errorf("%d raw_capture original(s) survived the erasure of the activity they duplicate — "+
			"the content is one join away, and SAR exports raw_capture by email match", originals)
	}
	if provenance != 0 {
		t.Errorf("%d field_provenance row(s) survived, still naming who captured the erased body "+
			"and from where", provenance)
	}
}

// TestAPassMissingItsPurgerRefusesBeforeDestroyingAnything holds the half of the
// fix a passing sweep cannot show. The constructor is what stops a purger-less
// service being built; this is what happens if one is anyway, and the two things
// it asserts are the ones that make the refusal safe rather than merely loud.
//
// It refuses with the sentinel — not some other failure this test would have
// accepted as proof.
//
// And it refuses having destroyed NOTHING. A destructive pass that discovered a
// missing dependency partway would abort with earlier records already erased and
// every later policy unrun, nightly: the failure mode retentionActions' own
// comment is written to avoid. So the aged-out activity must still be intact
// after the refusal, which is only true while the check precedes the work.
func TestAPassMissingItsPurgerRefusesBeforeDestroyingAnything(t *testing.T) {
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

	// nil, spelled out: the argument the constructor now demands, supplied as
	// the value compose never passes.
	bare := privacy.NewRetentionService(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	err := bare.EvaluateInstallation(RetentionPassCtx(e.WS))
	if !errors.Is(err, privacy.ErrRetentionSeamMissing) {
		t.Fatalf("a pass with no raw-capture purger returned %v, want ErrRetentionSeamMissing — "+
			"it must refuse, because the provider original has no other way to age out", err)
	}

	var body *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT body FROM activity WHERE subject = 'Quarterly figures'`).Scan(&body)
	}); err != nil {
		t.Fatalf("reading the record after the refusal: %v", err)
	}
	if body == nil || *body != "the numbers themselves" {
		t.Error("the refused pass had already erased the activity's body — a pass that cannot finish " +
			"must destroy nothing, or it leaves the installation half-swept with no record of where it stopped")
	}
}
