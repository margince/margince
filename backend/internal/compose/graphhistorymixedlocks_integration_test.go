// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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

func TestAMixedGraphBatchLocksItsHistoricalTargetsBeforeItsFirstFold(t *testing.T) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	var contactsIDs []ids.UUID
	for _, name := range []string{"Live correspondent", "History correspondent"} {
		contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		contactsIDs = append(contactsIDs, ids.UUID(contact.Id))
	}
	gen := NewGraphEdgeConsumer(b.env.Pool)
	var arrivals []events.Envelope
	for i, contact := range contactsIDs {
		source := b.env.Admin()
		if i == 1 {
			source = storekit.WithBatch(source, run)
		}
		first := logHistoryGraphMail(source, t, b, []ids.UUID{contact}, clocktest.Now(t).Add(-48*time.Hour))
		if err := gen.HandleEvent(context.Background(), latestDerivedEvent(t, b.env, "activity.captured", first, "")); err != nil {
			t.Fatal(err)
		}
		second := logHistoryGraphMail(source, t, b, []ids.UUID{contact}, clocktest.Now(t).Add(-24*time.Hour))
		arrivals = append(arrivals, latestDerivedEvent(t, b.env, "activity.captured", second, ""))
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "system:graph_edge_reconcile")
	ctx, cancel := context.WithTimeout(system, 10*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	config := b.env.Pool.Config()
	config.ConnConfig.Tracer = &graphMarkBarrier{entered: entered, release: release, marker: "UPDATE graph_interaction_edge e SET requested_generation"}
	pool, err := testdb.OwnPoolFromConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	result, done := make(chan error, 1), make(chan struct{})
	go func() { defer close(done); result <- NewGraphEdgeConsumer(pool).HandleBatch(ctx, arrivals) }()
	t.Cleanup(func() { cancel(); unblock(); <-done })
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("batch did not reach historical work: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	err = database.WithWorkspaceTx(ctx, b.env.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `SELECT contact_id FROM graph_interaction_edge WHERE contact_id=@contact FOR UPDATE NOWAIT`, pgx.NamedArgs{"contact": contactsIDs[1]})
		return err
	})
	blocked, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || blocked.Code != "55P03" {
		t.Fatalf("historical target was not locked with the immediate target: %v", err)
	}
	unblock()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTerminalImportWork(system, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { _, err := search.DrainPendingGraphTx(system, tx, 100); return err }); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE count_total=2`); got != 2 {
		t.Fatalf("mixed batch left %d complete edges", got)
	}
}
