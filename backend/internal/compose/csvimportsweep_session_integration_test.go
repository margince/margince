// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedDraft uploads a source the way the upload does, intent before bytes,
// and opens a draft on it.
func storedDraft(t *testing.T, e *integration.Env, blob blobstore.Store) migration.Run {
	t.Helper()
	ctx := importAdmin(e)
	key := "ws/import/" + ids.NewV7().String()
	runs := migration.NewRunStore(e.DB())
	if err := runs.RecordImportSourceIntent(ctx, key); err != nil {
		t.Fatalf("RecordImportSourceIntent: %v", err)
	}
	const body = "Email\nada@lovelace.example\n"
	if err := blob.Put(ctx, key, strings.NewReader(body), int64(len(body)), "text/csv"); err != nil {
		t.Fatalf("storing the source: %v", err)
	}
	draft, err := runs.CreateDraftRun(ctx, migration.CreateDraftRunInput{
		SourceRef: key, Source: importSourceProvenance, FileName: "leads.csv", Profile: []byte(`{"files":[]}`),
	})
	if err != nil {
		t.Fatalf("CreateDraftRun: %v", err)
	}
	return draft
}

// leaveIdle makes the run's last write as old as age.
func leaveIdle(t *testing.T, e *integration.Env, id migration.RunID, age time.Duration) {
	t.Helper()
	ctx := importAdmin(e)
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE import_run SET updated_at = now() - make_interval(secs => $2) WHERE id = $1`,
			id, age.Seconds())
		return err
	}); err != nil {
		t.Fatalf("ageing run %s: %v", id, err)
	}
}

func sweepOnce(t *testing.T, e *integration.Env) {
	t.Helper()
	worker := &importRunSweepWorker{identity: identity.NewService(e.Pool), pool: e.Pool, log: slog.New(slog.DiscardHandler)}
	if err := worker.Work(context.Background(), &river.Job[ImportRunSweepArgs]{JobRow: &rivertype.JobRow{Attempt: 1}}); err != nil {
		t.Fatalf("one sweep pass: %v", err)
	}
}

// reapPastGrace runs the stored-object reaper as if the grace period had
// passed, on the database clock that dated the ledger.
func reapPastGrace(t *testing.T, e *integration.Env, blob blobstore.Store) {
	t.Helper()
	ctx := importAdmin(e)
	var dbNow time.Time
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error { return tx.QueryRow(ctx, `SELECT now()`).Scan(&dbNow) }); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	later := func() time.Time { return dbNow.Add(2 * storedobjects.ProvisionalObjectGrace) }
	reaper := &storedObjectReapWorker{pool: e.Pool, blob: blob, log: slog.New(slog.DiscardHandler), now: later}
	if err := reaper.reapWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("reapWorkspace: %v", err)
	}
}

func sourceKept(t *testing.T, blob blobstore.Store, key string) bool {
	t.Helper()
	r, _, err := blob.Get(context.Background(), key)
	if errors.Is(err, blobstore.ErrNotFound) {
		return false
	}
	if err != nil {
		t.Fatalf("reading %s: %v", key, err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("closing %s: %v", key, err)
	}
	return true
}

func TestAnIdleDraftIsCancelledAndItsSourceReaped(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	idle, fresh := storedDraft(t, e, blob), storedDraft(t, e, blob)
	leaveIdle(t, e, idle.ID, importDraftTTL+time.Hour)
	leaveIdle(t, e, fresh.ID, importDraftTTL-time.Hour)

	sweepOnce(t, e)
	reapPastGrace(t, e, blob)

	got, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), idle.ID)
	if err != nil {
		t.Fatalf("GetStaged: %v", err)
	}
	if got.Status != migration.StatusCancelled || !got.SourceReleased || got.Error != migration.CauseDraftExpired {
		t.Errorf("the idle draft reads %s, released %v, error %q; want cancelled, released, saying it expired",
			got.Status, got.SourceReleased, got.Error)
	}
	if sourceKept(t, blob, idle.SourceRef) {
		t.Error("the idle draft's file is still stored after the reaper's pass")
	}
	if status := statusOf(t, e, fresh.ID); status != migration.StatusPending || !sourceKept(t, blob, fresh.SourceRef) {
		t.Errorf("a draft inside its time to live reads %s, file kept %v; want it pending with its file",
			status, sourceKept(t, blob, fresh.SourceRef))
	}
}
