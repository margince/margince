// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type graphFoldBarrierTx struct {
	pgx.Tx
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
	fold    string
	observe func([]any)
}

func (tx *graphFoldBarrierTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	isFold := strings.Contains(sql, tx.fold)
	if isFold && tx.observe != nil {
		tx.observe(args)
	}
	rows, err := tx.Tx.Query(ctx, sql, args...)
	if err != nil || !isFold {
		return rows, err
	}
	return &graphFoldBarrierRows{Rows: rows, boundary: tx, ctx: ctx}, nil
}

type graphFoldBarrierRows struct {
	pgx.Rows
	boundary *graphFoldBarrierTx
	ctx      context.Context
}

func (rows *graphFoldBarrierRows) Close() {
	rows.Rows.Close()
	rows.boundary.once.Do(func() {
		close(rows.boundary.entered)
		select {
		case <-rows.boundary.release:
		case <-rows.ctx.Done():
		}
	})
}

func TestALateHistoricalEventRemainsPendingAfterAConcurrentGraphDrain(t *testing.T) {
	for _, test := range []struct {
		name, fold string
		parties    int
	}{
		{"contact pair", "INSERT INTO graph_contact_edge AS e", 2},
		{"interaction pair", "INSERT INTO graph_interaction_edge AS e", 1},
	} {
		t.Run(test.name, func(t *testing.T) { assertLateHistorySurvivesDrain(t, test.fold, test.parties) })
	}
}

func assertLateHistorySurvivesDrain(t *testing.T, fold string, parties int) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	sourceCtx := storekit.WithBatch(b.env.Admin(), run)
	var contactIDs []ids.UUID
	for _, name := range []string{"Concurrent sender", "Concurrent peer"}[:parties] {
		contact, err := b.env.Contacts.CreateContact(sourceCtx, contacts.CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		contactIDs = append(contactIDs, ids.UUID(contact.Id))
	}
	at := clocktest.Now(t).Add(-48 * time.Hour)
	logHistoryGraphMail(sourceCtx, t, b, contactIDs, at)
	gen := NewGraphEdgeConsumer(b.env.Pool)
	first := historyGraphEvents(t, b, run)
	if len(first) != 1 {
		t.Fatalf("first events=%d", len(first))
	}
	if err := gen.HandleEvent(context.Background(), first[0]); err != nil {
		t.Fatal(err)
	}
	logHistoryGraphMail(sourceCtx, t, b, contactIDs, at.Add(24*time.Hour))
	all := historyGraphEvents(t, b, run)
	if len(all) != 2 {
		t.Fatalf("all events=%d", len(all))
	}
	if err := gen.HandleEvent(context.Background(), all[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "system:graph_edge_reconcile")
	if _, err := releaseTerminalImportWork(system, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(system, 10*time.Second)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	drained, drainDone := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(drainDone)
		drained <- database.WithWorkspaceTx(ctx, b.env.Pool, func(tx pgx.Tx) error {
			_, err := search.DrainPendingGraphTx(ctx, &graphFoldBarrierTx{Tx: tx, entered: entered, release: release, fold: fold}, 100)
			return err
		})
	}()
	t.Cleanup(func() { cancel(); unblock(); <-drainDone })
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	logHistoryGraphMail(sourceCtx, t, b, contactIDs, at.Add(48*time.Hour))
	late := historyGraphEvents(t, b, run)
	if len(late) != 3 {
		t.Fatalf("late events=%d", len(late))
	}
	marking, markDone, observed := make(chan error, 1), make(chan struct{}), make(chan struct{})
	gen.WithHistoryResolver(func(ctx context.Context, tx pgx.Tx, env events.Envelope) (*search.HistoryImport, error) {
		history, err := graphHistoryForEventTx(ctx, tx, env)
		close(observed)
		return history, err
	})
	go func() { defer close(markDone); marking <- gen.HandleEvent(ctx, late[2]) }()
	t.Cleanup(func() { cancel(); unblock(); <-markDone })
	select {
	case <-observed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	unblock()
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if err := <-marking; err != nil {
		t.Fatal(err)
	}
	wantInteractions := parties
	if strings.Contains(fold, "graph_contact_edge") {
		wantInteractions = 0
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE requested_generation>processed_generation AND deferred_run_id IS NULL`); got != wantInteractions {
		t.Fatalf("interaction successors=%d want=%d", got, wantInteractions)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_contact_edge WHERE requested_generation>processed_generation AND deferred_run_id IS NULL`); got != parties*(parties-1)/2 {
		t.Fatalf("contact successors=%d", got)
	}
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { _, err := search.DrainPendingGraphTx(system, tx, 100); return err }); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE requested_generation>processed_generation`); got != 0 {
		t.Fatalf("settled interaction requests=%d", got)
	}
}
