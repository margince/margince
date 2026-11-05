// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type graphFoldObserver struct {
	marker  string
	started chan uint32
	once    sync.Once
}

func (o *graphFoldObserver) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, o.marker) {
		o.once.Do(func() { o.started <- conn.PgConn().PID() })
	}
	return ctx
}
func (*graphFoldObserver) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestAConcurrentFirstGraphFoldCannotHideANewerHistoricalArrival(t *testing.T) {
	for _, test := range []struct {
		name, table string
		parties     int
	}{
		{"contact pair", "graph_contact_edge", 2},
		{"interaction pair", "graph_interaction_edge", 1},
	} {
		t.Run(test.name, func(t *testing.T) { assertHistorySurvivesFirstGraphFold(t, test.table, test.parties) })
	}
}

func assertHistorySurvivesFirstGraphFold(t *testing.T, table string, parties int) {
	b := setupBackfillWire(t)
	firstRun := strandedBackfill(t, b)
	other, secondRun := startSecondHistory(t, b)
	dispatch := backfillWorkContext(principal.WithWorkspaceID(context.Background(), b.env.WS), t, b)
	if err := b.registry.DispatchBackfills(dispatch, backfillScheduler(b.env.WS), false); err != nil {
		t.Fatal(err)
	}
	var allContacts []ids.UUID
	for i := range parties * 2 {
		contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: fmt.Sprintf("Insert correspondent %d", i), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		allContacts = append(allContacts, ids.UUID(contact.Id))
	}
	slices.SortFunc(allContacts, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
	blockers, targets := allContacts[:parties], allContacts[parties:]
	oldSource := storekit.WithBatch(b.env.Admin(), secondRun)
	blocker := logHistoryGraphMail(oldSource, t, b, blockers, clocktest.Now(t).Add(-72*time.Hour))
	first := logHistoryGraphMail(oldSource, t, b, targets, clocktest.Now(t).Add(-48*time.Hour))
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "system:graph_edge_reconcile")
	ctx, cancel := context.WithTimeout(system, 10*time.Second)
	defer cancel()
	commitBlocker := holdGraphRecompute(ctx, t, b.env.Pool, []ids.UUID{blocker})
	observer := &graphFoldObserver{marker: "INSERT INTO " + table + " AS e", started: make(chan uint32, 1)}
	config := b.env.Pool.Config()
	config.ConnConfig.Tracer = observer
	olderPool, err := testdb.OwnPoolFromConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(olderPool.Close)
	oldEvents := []events.Envelope{latestDerivedEvent(t, b.env, "activity.captured", blocker, ""), latestDerivedEvent(t, b.env, "activity.captured", first, "")}
	olderResult, olderDone := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(olderDone)
		olderResult <- NewGraphEdgeConsumer(olderPool).HandleBatch(ctx, oldEvents)
	}()
	t.Cleanup(func() { cancel(); commitBlocker(); <-olderDone })
	select {
	case pid := <-observer.started:
		awaitBlockedGraphFold(ctx, t, b.env.Pool, pid)
	case err := <-olderResult:
		t.Fatalf("older fold did not start: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second := logHistoryGraphMail(storekit.WithBatch(b.env.Admin(), firstRun), t, b, targets, clocktest.Now(t).Add(-24*time.Hour))
	latest := latestDerivedEvent(t, b.env, "activity.captured", second, "")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	config = b.env.Pool.Config()
	config.ConnConfig.Tracer = &graphMarkBarrier{entered: entered, release: release, marker: "LEFT JOIN " + table + " e ON"}
	newerPool, err := testdb.OwnPoolFromConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(newerPool.Close)
	newerResult, newerDone := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(newerDone)
		newerResult <- NewGraphEdgeConsumer(newerPool).HandleEvent(ctx, latest)
	}()
	t.Cleanup(func() { cancel(); unblock(); commitBlocker(); <-newerDone })
	select {
	case <-entered:
	case err := <-newerResult:
		t.Fatalf("newer arrival did not reach coverage check: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	commitBlocker()
	if err := <-olderResult; err != nil {
		t.Fatal(err)
	}
	unblock()
	if err := <-newerResult; err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.CancelBackfill(other, "gmail", ids.From[ids.UserKind](b.env.Rep2)); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTerminalImportWork(system, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { _, err := search.DrainPendingGraphTx(system, tx, 100); return err }); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE count_total=2 AND requested_generation=processed_generation`); got != parties {
		t.Fatalf("first fold lost interaction arrivals: %d", got)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_contact_edge WHERE count_total=2 AND requested_generation=processed_generation`); got != parties*(parties-1)/2 {
		t.Fatalf("first fold lost contact arrivals: %d", got)
	}
}

func awaitBlockedGraphFold(ctx context.Context, t *testing.T, pool *pgxpool.Pool, pid uint32) {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	for {
		if _, err := conn.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		var blocked bool
		if err := conn.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids(@pid))>0`, pgx.NamedArgs{"pid": pid}).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
	}
}
