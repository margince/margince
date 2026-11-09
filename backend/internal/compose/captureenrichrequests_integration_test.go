// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestHistorySignatureRequestsSurviveDelayedDeliveryAndCancellation(t *testing.T) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	ctx := storekit.WithBatch(b.env.Admin(), run)
	contact, err := b.env.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Historical correspondent", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	var encoded []byte
	if err := database.WithWorkspaceTx(ctx, b.env.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT envelope FROM event_outbox WHERE envelope->>'type'='contact.created' AND envelope->'entity'->>'id'=@id`, pgx.NamedArgs{"id": contact.Id.String()}).Scan(&encoded)
	}); err != nil {
		t.Fatal(err)
	}
	var event events.Envelope
	if err := json.Unmarshal(encoded, &event); err != nil {
		t.Fatal(err)
	}

	inserter, err := jobs.NewInserter(b.env.Pool, b.handlers.log)
	if err != nil {
		t.Fatal(err)
	}
	trigger := NewCaptureEnrichTrigger(b.env.Pool, inserter, b.handlers.log)
	trigger.now = func() time.Time { return event.OccurredAt.Add(2 * captureEnrichFreshWindow) }
	if err := trigger.HandleEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "agent:enrich")
	store := newContactsStore(InstallationDB(b.env.Pool))
	work, err := store.PendingSignatureWork(system, 100)
	if err != nil || len(work) != 0 {
		t.Fatalf("active history work was not deferred: %+v %v", work, err)
	}
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTerminalImportWork(system, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	work, err = store.PendingSignatureWork(system, 100)
	if err != nil || len(work) != 1 {
		t.Fatalf("cancelled history work was lost: %+v %v", work, err)
	}
	if err := store.SettleSignatureWork(system, work[0], 0); err != nil {
		t.Fatal(err)
	}
	if err := trigger.HandleEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	work, err = store.PendingSignatureWork(system, 100)
	if err != nil || len(work) != 0 {
		t.Fatalf("covered terminal event reopened completed work: %+v %v", work, err)
	}
}

func TestHistoricalSignatureWorkCannotPostponeLiveMail(t *testing.T) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: "Shared correspondent", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), b.env.WS), "agent:enrich")
	queue := func(deferred *ids.UUID) {
		t.Helper()
		if err := database.WithWorkspaceTx(ctx, b.env.Pool, func(tx pgx.Tx) error { return contacts.QueueSignatureWorkTx(ctx, tx, []ids.ContactID{id}, deferred) }); err != nil {
			t.Fatal(err)
		}
	}
	store := newContactsStore(InstallationDB(b.env.Pool))
	queue(&run)
	work, err := store.PendingSignatureWork(ctx, 10)
	if err != nil || len(work) != 0 {
		t.Fatalf("active history work=%+v err=%v", work, err)
	}
	queue(nil)
	queue(&run)
	work, err = store.PendingSignatureWork(ctx, 10)
	if err != nil || len(work) != 1 {
		t.Fatalf("history postponed live mail: %+v err=%v", work, err)
	}
	if err := store.SettleSignatureWork(ctx, work[0], 0); err != nil {
		t.Fatal(err)
	}
	queue(&run)
	if _, err := b.registry.CancelBackfill(b.human, "gmail", ids.From[ids.UserKind](b.env.Rep1)); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseTerminalImportWork(ctx, b.env.Pool); err != nil {
		t.Fatal(err)
	}
	work, err = store.PendingSignatureWork(ctx, 10)
	if err != nil || len(work) != 1 {
		t.Fatalf("terminal release=%+v err=%v", work, err)
	}
}

func TestOtherImportedMailDoesNotRequestSignatureWork(t *testing.T) {
	b := setupBackfillWire(t)
	contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: "Imported correspondent", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	run := ids.NewV7()
	source := storekit.WithImportRun(storekit.WithBatch(b.env.Admin(), run), run)
	logHistoryGraphMail(source, t, b, []ids.UUID{ids.UUID(contact.Id)}, clocktest.Now(t))
	all := historyGraphEvents(t, b, run)
	if len(all) != 1 || !events.IsImported(all[0]) {
		t.Fatalf("actual import event=%+v", all)
	}
	inserter, err := jobs.NewInserter(b.env.Pool, b.handlers.log)
	if err != nil {
		t.Fatal(err)
	}
	trigger := NewCaptureEnrichTrigger(b.env.Pool, inserter, b.handlers.log)
	trigger.now = func() time.Time { return all[0].OccurredAt }
	if err := trigger.HandleEvent(context.Background(), all[0]); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM contact_signature_enrich_state`); got != 0 {
		t.Fatalf("other import queued %d contacts", got)
	}
}

func TestHistoryArrivingDuringAFailedSignatureReadDoesNotEraseBackoff(t *testing.T) {
	b := setupBackfillWire(t)
	run := strandedBackfill(t, b)
	contact, err := b.env.Contacts.CreateContact(b.env.Admin(), contacts.CreateContactInput{FullName: "Deferred correspondent", Source: "manual"})
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
	queue(nil)
	store := newContactsStore(InstallationDB(b.env.Pool))
	observed, err := store.PendingSignatureWork(system, 10)
	if err != nil || len(observed) != 1 {
		t.Fatalf("observed=%+v %v", observed, err)
	}
	queue(&run)
	if err := store.SettleSignatureWork(system, observed[0], 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := b.env.WsCount(t, `SELECT count(*) FROM contact_signature_enrich_state WHERE contact_id=@contact AND due_at>now()`, pgx.NamedArgs{"contact": id}); got != 1 {
		t.Fatal("historical arrival erased retry backoff")
	}
	queue(nil)
	pending, err := store.PendingSignatureWork(system, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("new live source was postponed: %+v %v", pending, err)
	}
}
