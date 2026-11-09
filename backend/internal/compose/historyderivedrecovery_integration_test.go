// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDurableHistoryWorkRecoversWithoutRetainingItsRiverJobs(t *testing.T) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	source := storekit.WithBatch(b.env.Admin(), run)
	contact, err := b.env.Contacts.CreateContact(source, contacts.CreateContactInput{FullName: "Recoverable correspondent", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "agent:enrich")
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error {
		return contacts.QueueSignatureWorkTx(system, tx, []ids.ContactID{ids.From[ids.ContactKind](ids.UUID(contact.Id))}, &run)
	}); err != nil {
		t.Fatal(err)
	}
	gen := NewGraphEdgeConsumer(b.env.Pool)
	for day := range 2 {
		logHistoryGraphMail(source, t, b, []ids.UUID{ids.UUID(contact.Id)}, clocktest.Now(t).Add(time.Duration(day-3)*24*time.Hour))
		all := historyGraphEvents(t, b, run)
		if err := gen.HandleEvent(context.Background(), all[len(all)-1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	ctx := backfillWorkContext(system, t, b)
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Insert(ctx, GraphEdgeReconcileArgs{}, oneOffPassOpts(GraphEdgeReconcileArgs{}.Kind())); err != nil {
		t.Fatal(err)
	}
	worker := &captureBackfillReconcileWorker{pool: b.env.Pool, log: b.handlers.log, enrichEnabled: true}
	recoverWork := func() {
		t.Helper()
		if err := worker.reconcileDerivedWork(ctx); err != nil {
			t.Fatal(err)
		}
	}
	count := func(kind string) int {
		t.Helper()
		args := []any{kind}
		var n int
		if err := b.env.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM river_job WHERE kind=$%d AND args->>'pending'='true'`, len(args)), args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	recoverWork()
	for _, kind := range []string{CaptureEnrichArgs{}.Kind(), GraphEdgeReconcileArgs{}.Kind()} {
		if got := count(kind); got != 1 {
			t.Fatalf("%s pending jobs=%d", kind, got)
		}
	}
	if _, err := b.env.Pool.Exec(ctx, `UPDATE river_job SET state='running',attempted_at=now() WHERE args->>'pending'='true'`); err != nil {
		t.Fatal(err)
	}
	recoverWork()
	for _, kind := range []string{CaptureEnrichArgs{}.Kind(), GraphEdgeReconcileArgs{}.Kind()} {
		if got := count(kind); got != 1 {
			t.Fatalf("%s running job did not coalesce: %d", kind, got)
		}
	}
	if _, err := b.env.Pool.Exec(ctx, `DELETE FROM river_job WHERE args->>'pending'='true'`); err != nil {
		t.Fatal(err)
	}
	recoverWork()
	for _, kind := range []string{CaptureEnrichArgs{}.Kind(), GraphEdgeReconcileArgs{}.Kind()} {
		if got := count(kind); got != 1 {
			t.Fatalf("%s lost durable work after job cleanup: %d", kind, got)
		}
	}
	graphWorker := newGraphEdgeReconcileWorker(b.env.Pool, b.handlers.log)
	if err := graphWorker.Work(ctx, &river.Job[GraphEdgeReconcileArgs]{Args: GraphEdgeReconcileArgs{Pending: true}}); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE requested_generation>processed_generation`); got != 0 {
		t.Fatalf("graph worker left %d requests", got)
	}
}

func TestTerminalReleaseAndPendingReadsStayBoundedAndSkipBusyContacts(t *testing.T) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	source := storekit.WithBatch(b.env.Admin(), run)
	contactsIDs := make([]ids.ContactID, 0, historyReleaseRowBatch+1)
	for i := range historyReleaseRowBatch + 1 {
		contact, err := b.env.Contacts.CreateContact(source, contacts.CreateContactInput{FullName: fmt.Sprintf("Historical correspondent %d", i), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		contactsIDs = append(contactsIDs, ids.From[ids.ContactKind](ids.UUID(contact.Id)))
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "agent:enrich")
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { return contacts.QueueSignatureWorkTx(system, tx, contactsIDs, &run) }); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(system, 10*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	go func() {
		done <- database.WithWorkspaceTx(ctx, b.env.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT contact_id FROM contact_signature_enrich_state WHERE contact_id=@contact FOR UPDATE`, pgx.NamedArgs{"contact": contactsIDs[0]}); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var waitOnce sync.Once
	wait := func() {
		waitOnce.Do(func() {
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(func() { unblock(); wait() })
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	jobctx := backfillWorkContext(system, t, b)
	recovery := &captureBackfillReconcileWorker{pool: b.env.Pool, log: b.handlers.log, enrichEnabled: true}
	if err := recovery.reconcileDerivedWork(jobctx); !isPendingSnooze(err) {
		t.Fatalf("full release did not continue: %v", err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE deferred_run_id IS NULL`); got != historyReleaseRowBatch {
		t.Fatalf("released %d contacts", got)
	}
	if err := recovery.reconcileDerivedWork(jobctx); err != nil {
		t.Fatalf("busy contact made recovery block or spin: %v", err)
	}
	unblock()
	wait()
	if err := recovery.reconcileDerivedWork(jobctx); err != nil {
		t.Fatal(err)
	}
	reader := NewCaptureEnricher(b.env.Pool, nil, b.handlers.log)
	worker := &captureEnrichWorker{pool: b.env.Pool, enricher: reader, log: b.handlers.log}
	for pass := range 3 {
		err := worker.Work(jobctx, &river.Job[CaptureEnrichArgs]{Args: CaptureEnrichArgs{Pending: true}})
		if pass < 2 && !isPendingSnooze(err) {
			t.Fatalf("pending pass %d did not continue: %v", pass, err)
		}
		if pass == 2 && err != nil {
			t.Fatal(err)
		}
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE requested_generation>processed_generation`); got != 0 {
		t.Fatalf("undrained contacts=%d", got)
	}
}

func isPendingSnooze(err error) bool {
	snooze, ok := errors.AsType[*river.JobSnoozeError](err)
	return ok && snooze.Duration == 0
}
