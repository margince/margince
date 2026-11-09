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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type graphMarkBarrier struct {
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
	marker  string
}

func (b *graphMarkBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, b.marker) {
		b.once.Do(func() { close(b.entered) })
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	}
	return ctx
}

func (*graphMarkBarrier) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestAConcurrentGraphDeletionCannotLoseAHistoricalArrival(t *testing.T) {
	for _, test := range []struct {
		name, table string
		parties     int
	}{
		{"contact pair", "graph_contact_edge", 2},
		{"interaction pair", "graph_interaction_edge", 1},
	} {
		t.Run(test.name, func(t *testing.T) { assertHistorySurvivesDeletedGraphRow(t, test.table, test.parties) })
	}
}

func assertHistorySurvivesDeletedGraphRow(t *testing.T, table string, parties int) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	source := storekit.WithBatch(b.env.Admin(), run)
	var contactIDs []ids.UUID
	for _, name := range []string{"First correspondent", "Second correspondent"}[:parties] {
		contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		contactIDs = append(contactIDs, ids.UUID(contact.Id))
	}
	first := logHistoryGraphMail(source, t, b, contactIDs, clocktest.Now(t).Add(-48*time.Hour))
	gen := NewGraphEdgeConsumer(b.env.Pool)
	initial := historyGraphEvents(t, b, run)
	if err := gen.HandleEvent(context.Background(), initial[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := b.env.Activities.ArchiveActivity(b.env.Admin(), ids.From[ids.ActivityKind](first), nil); err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "system:graph_edge_reconcile")
	ctx, cancel := context.WithTimeout(system, 10*time.Second)
	defer cancel()
	commitDelete := holdGraphRecompute(ctx, t, b.env.Pool, []ids.UUID{first})
	logHistoryGraphMail(source, t, b, contactIDs, clocktest.Now(t).Add(-24*time.Hour))
	all := historyGraphEvents(t, b, run)
	entered, release := make(chan struct{}), make(chan struct{})
	var markOnce sync.Once
	unblockMark := func() { markOnce.Do(func() { close(release) }) }
	tracer := &graphMarkBarrier{entered: entered, release: release, marker: "FROM targets t JOIN " + table + " e"}
	config := b.env.Pool.Config()
	config.ConnConfig.Tracer = tracer
	pool, err := testdb.OwnPoolFromConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	gen = NewGraphEdgeConsumer(pool)
	markResult, markDone := make(chan error, 1), make(chan struct{})
	go func() { defer close(markDone); markResult <- gen.HandleEvent(ctx, all[1]) }()
	t.Cleanup(func() { cancel(); unblockMark(); commitDelete(); <-markDone })
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	commitDelete()
	unblockMark()
	if err := <-markResult; err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE count_total=1`); got != parties {
		t.Fatalf("arrival lost interaction rows: %d", got)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_contact_edge WHERE count_total=1`); got != parties*(parties-1)/2 {
		t.Fatalf("arrival lost contact rows: %d", got)
	}
}

func holdGraphRecompute(ctx context.Context, t *testing.T, pool *pgxpool.Pool, activityIDs []ids.UUID) func() {
	t.Helper()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var workErr error
	go func() {
		defer close(done)
		workErr = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			if err := search.RecomputeEdgesForActivities(ctx, tx, activityIDs); err != nil {
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
	var once sync.Once
	commit := func() {
		once.Do(func() {
			close(release)
			<-done
			if workErr != nil {
				t.Error(workErr)
			}
		})
	}
	t.Cleanup(commit)
	select {
	case <-entered:
	case <-done:
		t.Fatal(workErr)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return commit
}
