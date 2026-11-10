// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestOverlappingHistoryImportsKeepSharedWorkDeferredAndLiveWorkPrompt(t *testing.T) {
	b := setupBackfillWire(t)
	b.registry.WithBackfillAdmission(2, false)
	first := strandedBackfill(t, b)
	other, second := startSecondHistory(t, b)
	dispatch := backfillWorkContext(principal.WithWorkspaceID(context.Background(), b.env.WS), t, b)
	if err := b.registry.DispatchBackfills(dispatch, backfillScheduler(b.env.WS), false); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM capture_backfill WHERE status='running'`); got != 2 {
		t.Fatalf("running imports=%d", got)
	}
	contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: "Shared correspondent", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "agent:enrich")
	queue := func(run *ids.UUID) {
		t.Helper()
		if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { return contacts.QueueSignatureWorkTx(system, tx, []ids.ContactID{id}, run) }); err != nil {
			t.Fatal(err)
		}
	}
	store := newContactsStore(InstallationDB(b.env.Pool))
	assertPending := func(want int) []contacts.SignatureWork {
		t.Helper()
		work, err := store.PendingSignatureWork(system, 100)
		if err != nil || len(work) != want {
			t.Fatalf("signature work=%+v want=%d err=%v", work, want, err)
		}
		return work
	}
	gen := NewGraphEdgeConsumer(b.env.Pool)
	for i, run := range []ids.UUID{first, first, second} {
		source := storekit.WithBatch(b.env.Admin(), run)
		logHistoryGraphMail(source, t, b, []ids.UUID{id.UUID}, clocktest.Now(t).Add(time.Duration(i-5)*24*time.Hour))
		all := historyGraphEvents(t, b, run)
		if err := gen.HandleEvent(context.Background(), all[len(all)-1]); err != nil {
			t.Fatal(err)
		}
		queue(&run)
	}
	if _, err := b.registry.CancelBackfill(other, "gmail", ids.From[ids.UserKind](b.env.Rep2)); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTerminalImportWork(system, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	assertPending(0)
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error {
		pending, err := search.HasPendingGraphWorkTx(system, tx)
		if pending {
			t.Fatal("one finished import released overlapping graph work")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	late := historyGraphEvents(t, b, second)
	if err := gen.HandleEvent(context.Background(), late[0]); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE deferred_run_id=@run`, pgx.NamedArgs{"run": first}); got != 1 {
		t.Fatal("terminal event did not retain the active cohort")
	}
	queue(nil)
	queue(&first)
	work := assertPending(1)
	if err := store.SettleSignatureWork(system, work[0], 0); err != nil {
		t.Fatal(err)
	}
	queue(&first)
	assertPending(0)
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTerminalImportWork(system, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	assertPending(1)
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { _, err := search.DrainPendingGraphTx(system, tx, 100); return err }); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE count_total=3 AND requested_generation=processed_generation`); got != 1 {
		t.Fatalf("final overlapping graph=%d", got)
	}
}

func startSecondHistory(t *testing.T, b *backfillWireEnv) (context.Context, ids.UUID) {
	t.Helper()
	b.registry.WithBackfillAdmission(2, false)
	actor, ok := principal.Actor(b.human)
	if !ok {
		t.Fatal("missing fixture actor")
	}
	actor.UserID = b.env.Rep2
	actor.ID = "human:" + b.env.Rep2.String()
	other := principal.WithActor(b.human, actor)
	if _, err := b.registry.Connect(other, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatal(err)
	}
	second, err := b.registry.StartBackfill(other, "gmail", ids.From[ids.UserKind](b.env.Rep2), 6, connector.BackfillEstimate{Messages: 25}, func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return other, second.ID
}
