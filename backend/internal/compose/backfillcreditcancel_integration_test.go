// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type cancelAfterCreation struct {
	delegate  capture.CounterpartyEnsurer
	cancel    func() error
	cancelErr error
	created   ids.UUID
}

// EnsureCounterpartyTx cancels the import once the creation has committed.
func (c *cancelAfterCreation) EnsureCounterpartyTx(ctx context.Context, tx pgx.Tx, in capture.EnsureRequest) (capture.EnsureOutcome, error) {
	outcome, err := c.delegate.EnsureCounterpartyTx(ctx, tx, in)
	if err == nil && outcome.ContactCreated {
		c.created, outcome.AfterCommit = outcome.ContactID, cancelAfter(outcome.AfterCommit, func() { c.cancelErr = c.cancel() })
	}
	return outcome, err
}

func cancelAfter(hook func(context.Context), then func()) func(context.Context) {
	return func(ctx context.Context) { hook(ctx); then() }
}

type singleRecordHistory struct {
	backfillFakeConnector
	records []connector.NormalizedRecord
	mailbox string
}

func (p *singleRecordHistory) AccountLabel(connector.Auth) (string, error) { return p.mailbox, nil }

func (p *singleRecordHistory) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, _ string, sink connector.Sink) (connector.BackfillPageResult, error) {
	for _, record := range p.records {
		if _, err := sink.Upsert(ctx, record); err != nil {
			return connector.BackfillPageResult{}, err
		}
	}
	return connector.BackfillPageResult{Scanned: len(p.records), Captured: len(p.records)}, nil
}

func TestACommittedCreationIsCreditedAfterItsImportIsCancelled(t *testing.T) {
	e := integration.Setup(t)
	record := partiesMailRecord(t, e, partyMailProof{outbound: true, sentByOwner: true}, "", "", "Please send the product requirements.")
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	ensurer := &cancelAfterCreation{delegate: contactsEnsurer{
		store: newCounterpartyStoreAt(e.Pool, func() time.Time { return clocktest.Now(t) }), pool: e.Pool,
		triage: newDomainTriageTrigger(e.Pool, quiet, true), log: quiet,
	}}
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(ensurer, capture.NewTransactionalList(nil, nil))
	registry := capture.NewRegistry(e.DB(), sink, identity.NewServiceFor(e.DB()), keyvault.NewMemory())
	registry.Register(&singleRecordHistory{name: "gmail", messages: 1, pageSize: 1, mailbox: seatAddress(t, e, e.AdminUser), records: []connector.NormalizedRecord{record, partiesMailRecord(t, e, partyMailProof{}, "", "", "Here are the product requirements.")}})
	human := e.Admin()
	actor, ok := principal.Actor(human)
	if !ok {
		t.Fatal("actor not bound")
	}
	actor.Scopes = principal.NewScopeSet(principal.ScopeRead)
	human = principal.WithActor(human, actor)
	if _, err := registry.Connect(human, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatal(err)
	}
	run, err := registry.StartBackfill(human, "gmail", ids.From[ids.UserKind](e.AdminUser), 3, connector.BackfillEstimate{Messages: 1}, func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ensurer.cancel = func() error {
		_, err := registry.CancelBackfill(human, "gmail", ids.From[ids.UserKind](e.AdminUser))
		return err
	}
	if _, _, _, err := registry.RunBackfillStep(human, run.ID); err != nil && !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal(err)
	}
	if ensurer.created == (ids.UUID{}) {
		t.Fatal("fixture created no contact")
	}
	if ensurer.cancelErr != nil {
		t.Fatalf("cancelling the import: %v", ensurer.cancelErr)
	}
	args := []any{ensurer.created}
	if e.WsCount(t, fmt.Sprintf(`SELECT count(*) FROM contact WHERE id=$%d`, len(args)), args...) != 1 {
		t.Fatal("creation was not committed")
	}
	var status string
	var count, credits int
	args = []any{run.ID}
	if err := e.Pool.QueryRow(human, fmt.Sprintf(`SELECT status,contacts_created,(SELECT count(*) FROM capture_backfill_creation WHERE backfill_id=b.id AND kind='contact') FROM capture_backfill b WHERE id=$%d`, len(args)), args...).Scan(&status, &count, &credits); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" || count != 1 || credits != 1 {
		t.Fatalf("status=%s contacts=%d credits=%d", status, count, credits)
	}
}
