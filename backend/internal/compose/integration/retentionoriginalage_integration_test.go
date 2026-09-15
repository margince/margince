// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The provider original ages out on ITS own clock.
//
// Until this stage, raw_capture was reached only by the activity sweep joined on
// the natural key and by Art. 17 erasure, so an installation whose activity
// policies are 1095d kept every verbatim original for three years — 92% of one
// measured database, from 12.6k rows over four mailboxes. Slimming the payload
// (#5163) cut the slope roughly twentyfold and left the growth unbounded.
//
// What the stage may destroy is bounded by the JOIN to the activity, and the
// four cases below are that boundary: the original of an ordinary aged message
// goes, the activity survives carrying the source key a replay tombstones
// against, a shielded Handelsbrief keeps its original however aggressive the
// policy, and an original with no activity is left alone because there the
// raw_capture row IS the tombstone.
//
// Driven through compose.NewRetentionServiceFor, like its sibling, because a
// test that supplies its own wiring proves nothing about the pass the worker
// runs.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheProviderOriginalAgesOutOnItsOwnClock(t *testing.T) {
	e := Setup(t)
	aged, recent, shielded := ids.NewV7(), ids.NewV7(), ids.NewV7()

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		// A raw_capture policy and NO activity policy. The independence is the
		// property: an installation keeps its correspondence for years and its
		// stored originals for months, which is the whole point of a clock of
		// its own.
		if _, err := tx.Exec(ctx, `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('raw_capture', NULL, 100, 'erase')`); err != nil {
			return err
		}
		seed := func(id ids.UUID, sourceID string, days string, class *string) error {
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by,
				                      source_system, source_id, retention_class, retention_class_at)
				VALUES ($1, 'email', 'Quarterly figures', 'the numbers themselves',
				        now() - $3::interval, 'capture', 'connector:t', 'gmail', $2, $4,
				        CASE WHEN $4::text IS NULL THEN NULL ELSE now() END)`,
				id, sourceID, days, class); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO raw_capture (source_system, source_id, payload, received_at)
				VALUES ('gmail', $1, $2, now() - $3::interval)`,
				sourceID, []byte(`{"body":"the numbers themselves"}`), days)
			return err
		}
		if err := seed(aged, "msg-aged", "400 days", nil); err != nil {
			return err
		}
		// Inside the policy's own window, so the stage must not reach it.
		if err := seed(recent, "msg-recent", "10 days", nil); err != nil {
			return err
		}
		// Stamped as commercial correspondence, which is the first arm of the
		// Handelsbrief test: §257 HGB obliges keeping it, and the original is
		// what it obliges keeping.
		commercial := "commercial_correspondence"
		if err := seed(shielded, "msg-shielded", "400 days", &commercial); err != nil {
			return err
		}
		// An original the pipeline dropped without writing an activity. Here the
		// raw_capture row is the ONLY tombstone against a replay re-ingesting a
		// message already judged, so the stage must leave it.
		_, err := tx.Exec(ctx, `
			INSERT INTO raw_capture (source_system, source_id, payload, received_at)
			VALUES ('gmail', 'msg-orphan', $1, now() - interval '400 days')`,
			[]byte(`{"body":"dropped before it became a record"}`))
		return err
	}); err != nil {
		t.Fatalf("seeding the originals: %v", err)
	}

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}

	originals := map[string]int{}
	var keptKey bool
	var body *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		rows, err := tx.Query(ctx, `
			SELECT source_id, count(*) FROM raw_capture GROUP BY source_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				return err
			}
			originals[id] = n
		}
		if err := rows.Err(); err != nil {
			return err
		}
		// The activity behind the destroyed original: still there, still
		// carrying the pair, and still holding its own text — this policy
		// governs the stored copy, not the record.
		return tx.QueryRow(ctx, `
			SELECT body, source_system = 'gmail' AND source_id = 'msg-aged'
			  FROM activity WHERE id = $1`, aged).Scan(&body, &keptKey)
	}); err != nil {
		t.Fatalf("reading the originals after the sweep: %v", err)
	}

	if originals["msg-aged"] != 0 {
		t.Errorf("the aged original survived its own policy (%d row(s)) — raw_capture is back to growing "+
			"until an activity policy happens to reach it", originals["msg-aged"])
	}
	if originals["msg-recent"] != 1 {
		t.Errorf("an original inside the policy's window was destroyed (%d row(s) left)",
			originals["msg-recent"])
	}
	if originals["msg-shielded"] != 1 {
		t.Errorf("the original of a Handelsbrief was destroyed (%d row(s) left) — the statutory floor "+
			"obliges keeping the correspondence, and the verbatim original is what it obliges keeping",
			originals["msg-shielded"])
	}
	if originals["msg-orphan"] != 1 {
		t.Errorf("an original with no activity was destroyed (%d row(s) left) — that row is the only "+
			"tombstone against a replay re-ingesting a message the pipeline already judged",
			originals["msg-orphan"])
	}
	if !keptKey {
		t.Error("the stage dropped the activity's (source_system, source_id) — that pair is what " +
			"tombstones a replay, and destroying the original must not cost it")
	}
	if body == nil {
		t.Error("the stage cleared the activity's own body — it governs the stored original, not the record")
	}
}
