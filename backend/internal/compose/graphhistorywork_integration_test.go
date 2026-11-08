// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
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

func historyGraphEvents(t *testing.T, b *backfillWireEnv, run ids.UUID) []events.Envelope {
	t.Helper()
	var eventsFound []events.Envelope
	if err := database.WithWorkspaceTx(b.env.Admin(), b.env.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT o.envelope FROM event_outbox o JOIN audit_log a
   ON a.id=(o.envelope->'trace'->>'audit_log_id')::uuid WHERE a.batch_id=@run
   AND o.envelope->>'type'='activity.captured' ORDER BY o.seq`, pgx.NamedArgs{"run": run})
		if err != nil {
			return err
		}
		eventsFound, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (events.Envelope, error) {
			var encoded []byte
			var event events.Envelope
			if err := row.Scan(&encoded); err != nil {
				return event, err
			}
			err := json.Unmarshal(encoded, &event)
			return event, err
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return eventsFound
}

func TestHistoricalGraphArrivalsCoalesceUntilTheImportEnds(t *testing.T) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	ctx := storekit.WithBatch(b.env.Admin(), run)
	contact, err := b.env.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "History correspondent", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	counterpart := ids.UUID(contact.Id)
	peer, err := b.env.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "History peer", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	peerID := ids.UUID(peer.Id)
	at := clocktest.Now(t).Add(-40 * 24 * time.Hour)
	logMail := func(day int) {
		logHistoryGraphMail(ctx, t, b, []ids.UUID{counterpart, peerID}, at.Add(time.Duration(day)*24*time.Hour))
	}
	logMail(0)
	observer := &graphFoldObserver{marker: "INSERT INTO graph_", started: make(chan uint32, 1)}
	config := b.env.Pool.Config()
	config.ConnConfig.Tracer = observer
	graphPool, err := testdb.OwnPoolFromConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(graphPool.Close)
	gen := NewGraphEdgeConsumer(graphPool)
	first := historyGraphEvents(t, b, run)
	if len(first) != 1 {
		t.Fatalf("captured events=%d want=1", len(first))
	}
	if events.IsImported(first[0]) {
		t.Fatal("mail history changed the existing bulk-import side-effect policy")
	}
	if err := gen.HandleEvent(context.Background(), first[0]); err != nil {
		t.Fatal(err)
	}
	if reads := observer.folds.Load(); reads != 4 {
		t.Fatalf("new pairs needed %d folds, want four across two tables", reads)
	}
	b.env.WsExec(t, `UPDATE graph_interaction_edge SET computed_at='2000-01-01T00:00:00Z' WHERE contact_id=@contact`, pgx.NamedArgs{"contact": counterpart})
	for day := 1; day < 32; day++ {
		logMail(day)
	}
	all := historyGraphEvents(t, b, run)
	if len(all) != 32 {
		t.Fatalf("captured events=%d want=32", len(all))
	}
	if err := gen.HandleBatch(context.Background(), all[1:]); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE contact_id=@contact AND count_total=1 AND computed_at='2000-01-01T00:00:00Z' AND requested_generation>processed_generation AND deferred_run_id=@run`, pgx.NamedArgs{"contact": counterpart, "run": run}); got != 1 {
		t.Fatalf("historical arrivals refolded instead of coalescing: matching rows=%d", got)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_contact_edge WHERE count_total=1 AND deferred_run_id=@run AND requested_generation>processed_generation`, pgx.NamedArgs{"run": run}); got != 1 {
		t.Fatalf("contact pair was not coalesced: %d", got)
	}
	if reads := observer.folds.Load(); reads != 4 {
		t.Fatalf("history repeated exact folds: %d", reads)
	}
	// Archival must remove evidence while the import still owns its deferred work.
	if _, err := b.env.Activities.ArchiveActivity(b.env.Admin(), ids.From[ids.ActivityKind](first[0].Entity.ID), nil); err != nil {
		t.Fatal(err)
	}
	if err := gen.HandleEvent(context.Background(), envelopeFor(b.env.WS, "activity.archived", "activity", first[0].Entity.ID)); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE contact_id=@contact AND count_total=31 AND computed_at<>'2000-01-01T00:00:00Z'`, pgx.NamedArgs{"contact": counterpart}); got != 1 {
		t.Fatalf("archival waited for the history drain: %d", got)
	}
	if reads := observer.folds.Load(); reads != 6 {
		t.Fatalf("existing-pair removal needed %d extra folds, want two", reads-4)
	}
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "system:graph_edge_reconcile")
	if _, err := releaseTerminalImportWork(system, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(system, graphPool, func(tx pgx.Tx) error {
		more, err := search.DrainPendingGraphTx(system, tx, 100)
		if more {
			return errors.New("one pair unexpectedly filled the drain cohort")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE contact_id=@contact AND count_total=31 AND requested_generation=processed_generation AND deferred_run_id IS NULL`, pgx.NamedArgs{"contact": counterpart}); got != 1 {
		t.Fatalf("drained graph row=%d want=1", got)
	}
	if err := gen.HandleBatch(context.Background(), all); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM graph_interaction_edge WHERE requested_generation>processed_generation`); got != 0 {
		t.Fatalf("covered backlog reopened %d pairs", got)
	}
	if reads := observer.folds.Load(); reads != 8 {
		t.Fatalf("drain and redelivery needed %d extra folds, want two", reads-6)
	}
	v := edgeEnv{b.env}
	drained := v.snapshot(t)
	if err := database.WithWorkspaceTx(system, b.env.Pool, func(tx pgx.Tx) error { return search.RebuildEdges(system, tx) }); err != nil {
		t.Fatal(err)
	}
	if rebuilt := v.snapshot(t); !equalSnapshots(drained, rebuilt) {
		t.Fatalf("drained=%v rebuilt=%v", drained, rebuilt)
	}
}

func logHistoryGraphMail(ctx context.Context, t *testing.T, b *backfillWireEnv, contacts []ids.UUID, when time.Time) ids.UUID {
	t.Helper()
	direction, subject := "inbound", "Historic mail"
	links := make([]activities.ActivityLinkInput, 0, len(contacts))
	for _, id := range contacts {
		links = append(links, activities.ActivityLinkInput{EntityType: "contact", EntityID: id})
	}
	activity, _, err := b.env.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "email", Direction: &direction, Subject: &subject, OccurredAt: &when, Source: "manual", Links: links})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(activity.Id)
}
