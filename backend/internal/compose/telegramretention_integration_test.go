// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A channel message's provider original is destroyed when the retention sweep
// erases the message, and only a run through the REAL ingress can say so.
//
// The sweep found the original by joining raw_capture to activity on
// (source_system, source_id). raw_capture has two writers and they key it
// differently — the mail sink by the domain natural key, the poller by the
// provider's redelivery key — so for a channel capture that join matched zero
// rows, deleted nothing, and reported success. The verbatim update, holding the
// message text and the sender's numeric id, username and names, survived every
// retention policy and came back in an Art. 15 package.
//
// The existing retention case seeds raw_capture with a literal INSERT under a
// mail-shaped key, so it agrees with the join by construction and could never
// have seen this. Here the poller writes the raw row and the sink writes the
// activity, which is the only arrangement in which their keys are allowed to
// disagree.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheRetentionSweepDestroysAChannelMessagesOriginal(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	vault := keyvault.NewMemory()
	api := &telegramPollFakeAPI{
		bot:  telegram.Bot{ID: 92000042, Username: "retention_bot"},
		held: []json.RawMessage{telegramPrivateUpdate(9001, 770001, 55, "the numbers themselves")},
	}
	conn := connectTestTelegramBot(t, e, vault, api, 92000042, "retention_bot")

	// The poll stores the original and enqueues the ingest; the ingest hands
	// the record to the real Sink, which writes the activity. Two writers, two
	// keys — the arrangement the natural-key join could not span.
	if err := runOnePoll(t, newTestPollWorker(e, vault, api, ambientPollInserter(t, e)), e.WS, conn); err != nil {
		t.Fatalf("polling: %v", err)
	}
	rawIDs := telegramEnqueuedRawIDs(t, e, conn.ID.String())
	if len(rawIDs) != 1 {
		t.Fatalf("the poll enqueued %d ingest jobs, want 1", len(rawIDs))
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	workOneIngestJob(t, e, &telegramIngestWorker{pool: e.Pool, sink: capture.NewSink(e.DB()), log: quiet}, rawIDs[0])

	activity := theCapturedChannelActivity(t, e)
	// The link, asserted BEFORE the sweep. Without it the sweep below could
	// pass for the wrong reason — an empty raw_capture proves nothing if the
	// poll never stored a row in the first place.
	if storedOriginalOf(t, e, activity).IsZero() {
		t.Fatal("the captured activity names no original, so nothing joins it to the row holding the text")
	}
	if n := rawCaptureRowsLeft(t, e); n != 1 {
		t.Fatalf("%d originals before the sweep, want the one the poll stored", n)
	}

	ageOutAndSweep(t, e, activity)

	if body := activityBodyOf(t, e, activity); body != nil {
		t.Fatalf("the sweep left the message body %q", *body)
	}
	if n := rawCaptureRowsLeft(t, e); n != 0 {
		t.Fatalf("%d provider original(s) survived the sweep — the verbatim update is still "+
			"there for an Art. 15 package to return", n)
	}
}

// theCapturedChannelActivity is the one activity the ingest wrote. Asserted as
// exactly one: a second would mean the run captured something this case did not
// arrange, and every read below would then be about an arbitrary row.
func theCapturedChannelActivity(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	var ids []ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT id FROM activity WHERE source_system = 'telegram'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		return scanUUIDs(rows, &ids)
	}); err != nil {
		t.Fatalf("reading the captured activity: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("%d telegram activities after one update, want 1", len(ids))
	}
	return ids[0]
}

func scanUUIDs(rows pgx.Rows, out *[]ids.UUID) error {
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		*out = append(*out, id)
	}
	return rows.Err()
}

// storedOriginalOf reads the link the purge follows.
func storedOriginalOf(t *testing.T, e *integration.Env, activity ids.UUID) ids.UUID {
	t.Helper()
	var raw *ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT raw_capture_id FROM activity WHERE id = $1`, activity).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the stored original: %v", err)
	}
	if raw == nil {
		return ids.Nil
	}
	return *raw
}

func activityBodyOf(t *testing.T, e *integration.Env, activity ids.UUID) *string {
	t.Helper()
	var body *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT body FROM activity WHERE id = $1`, activity).Scan(&body)
	}); err != nil {
		t.Fatalf("reading the message body: %v", err)
	}
	return body
}

func rawCaptureRowsLeft(t *testing.T, e *integration.Env) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM raw_capture`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the provider originals: %v", err)
	}
	return n
}

// ageOutAndSweep puts the message behind an erase policy's window and runs the
// real sweep over it, through the constructor jobs.go composes.
//
// The occurrence is moved rather than the policy being written short: the
// window is measured from occurred_at, and a zero-day policy would also sweep
// anything else this fixture happened to leave behind.
func ageOutAndSweep(t *testing.T, e *integration.Env, activity ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `
			INSERT INTO retention_policy (object_type, category, retain_days, action)
			VALUES ('activity', NULL, 100, 'erase')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE activity SET occurred_at = now() - interval '400 days' WHERE id = $1`, activity)
		return err
	}); err != nil {
		t.Fatalf("ageing the message out: %v", err)
	}
	svc := NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	integration.SettleIntoInstall(t, e)
	if err := svc.EvaluateInstallation(integration.RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}
}
