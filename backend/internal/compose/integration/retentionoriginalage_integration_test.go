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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// mailCaptureCtx is the connector principal a mail sync captures under —
// the sink door admitRecord requires, and the one every mail activity in
// this suite is seeded through now that the selector reads the reference
// only the sink writes.
func mailCaptureCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector,
		ID:   "connector:t",
		Permissions: principal.Permissions{
			RoleKeys: []string{"connector"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func TestTheProviderOriginalAgesOutOnItsOwnClock(t *testing.T) {
	e := Setup(t)
	sink := capture.NewSink(e.DB())

	// A raw_capture policy and NO activity policy. The independence is the
	// property: an installation keeps its correspondence for years and its
	// stored originals for months, which is the whole point of a clock of
	// its own.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('raw_capture', NULL, 100, 'erase')`)
		return err
	}); err != nil {
		t.Fatalf("seeding the policy: %v", err)
	}

	// seed captures one message through the real mail sink, which is what
	// gives the activity the raw_capture_id reference the age-out selector
	// now joins on — a literal INSERT into both tables never sets it. The
	// clock this stage reads (raw_capture.received_at) and the classification
	// the Handelsbrief floor reads (retention_class) are stamped afterward:
	// neither is a capture-time fact the sink's own record carries.
	seed := func(sourceID string, age time.Duration, class *string) error {
		ref, err := sink.Upsert(mailCaptureCtx(e.WS), connector.NormalizedRecord{
			EntityType: datasource.EntityActivity,
			NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
			Fields: capture.ActivityFields{
				Kind: "email", Subject: "Quarterly figures", Body: "the numbers themselves",
				OccurredAt: time.Now().Add(-age), Direction: connector.DirectionInbound,
			},
			Raw:        []byte(`{"body":"the numbers themselves"}`),
			Source:     "email:" + sourceID,
			CapturedBy: "connector:t",
			Counterparty: connector.Counterparty{
				Direction: connector.DirectionInbound,
				Email:     sourceID + "@retention.test",
				Domain:    "retention.test",
			},
		})
		if err != nil {
			return err
		}
		return database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			ctx := context.Background()
			if _, err := tx.Exec(ctx, `
				UPDATE raw_capture SET received_at = now() - make_interval(secs => $2)
				 WHERE id = (SELECT raw_capture_id FROM activity WHERE id = $1)`,
				ref.ID, age.Seconds()); err != nil {
				return err
			}
			if class == nil {
				return nil
			}
			_, err := tx.Exec(ctx, `
				UPDATE activity SET retention_class = $2, retention_class_at = now() WHERE id = $1`,
				ref.ID, *class)
			return err
		})
	}
	if err := seed("msg-aged", 400*24*time.Hour, nil); err != nil {
		t.Fatalf("seeding the aged message: %v", err)
	}
	// Inside the policy's own window, so the stage must not reach it.
	if err := seed("msg-recent", 10*24*time.Hour, nil); err != nil {
		t.Fatalf("seeding the recent message: %v", err)
	}
	// Stamped as commercial correspondence, which is the first arm of the
	// Handelsbrief test: §257 HGB obliges keeping it, and the original is
	// what it obliges keeping.
	commercial := "commercial_correspondence"
	if err := seed("msg-shielded", 400*24*time.Hour, &commercial); err != nil {
		t.Fatalf("seeding the shielded message: %v", err)
	}
	// An original the pipeline dropped without writing an activity. Here the
	// raw_capture row is the ONLY tombstone against a replay re-ingesting a
	// message already judged, so the stage must leave it — and nothing but a
	// literal INSERT can produce that shape, because it is exactly the row a
	// reference never gets to name.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO raw_capture (source_system, source_id, payload, received_at)
			VALUES ('gmail', 'msg-orphan', $1, now() - interval '400 days')`,
			[]byte(`{"body":"dropped before it became a record"}`))
		return err
	}); err != nil {
		t.Fatalf("seeding the orphaned original: %v", err)
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
			SELECT body, source_system = $1 AND source_id = 'msg-aged'
			  FROM activity WHERE source_id = 'msg-aged'`, connector.EmailSourceSystem).Scan(&body, &keptKey)
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
