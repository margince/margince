// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A stop recorded while an anonymize runs. Both take the subject's stop lock,
// so the stop either lands before the anonymize pins it, or after, onto the
// hashes the anonymize kept. Either way a later record with the same address
// is stopped.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// waitForStopLockWaiters waits, through testdb.WaitForContention, until n
// transactions wait on an advisory lock. returned closes when the racer that
// should be the n-th waiter returns without waiting.
func waitForStopLockWaiters(t *testing.T, n int, returned <-chan struct{}) {
	t.Helper()
	conn := integration.OwnerConn(t)
	testdb.WaitForContention(t, returned,
		"a racer returned without waiting on the stop lock, so nothing serialised it",
		fmt.Sprintf("fewer than %d transaction(s) waited on the stop lock in time", n),
		func(ctx context.Context) (bool, error) {
			var waiting int
			err := conn.QueryRow(ctx,
				`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`).Scan(&waiting)
			return waiting >= n, err
		})
}

// The anonymize queues first and the stop second, behind a held lock. The
// stop then finds the record anonymized and pins itself to the kept hash.
//
// Mutation: drop the stop lock from anonymiseOneContact, or the anonymized
// read from Suppress, and the returning address is mailable again.
func TestAStopRecordedDuringAnAnonymizeStillStopsTheAddress(t *testing.T) {
	e := integration.Setup(t)
	const address = "rosa.race@example.test"
	subject := e.SeedContact(t, "Rosa Race", &e.AdminUser)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	if _, err := owner.Exec(ctx, `
		INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'manual', 'human:x')`, subject, address); err != nil {
		t.Fatalf("seeding the address: %v", err)
	}

	hold, err := e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := storekit.LockStopKey(ctx, hold, subject.String()); err != nil {
		t.Fatal(err)
	}
	service := NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	anonymized, anonymizeReturned := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(anonymizeReturned)
		_, err := service.AnonymiseContacts(e.Admin(), []ids.UUID{subject}, privacy.PurgeOwnerRule, anonymiseAsChosen)
		anonymized <- err
	}()
	waitForStopLockWaiters(t, 1, anonymizeReturned)
	stopped, stopReturned := make(chan error, 1), make(chan struct{})
	store := consent.NewStore(e.DB())
	go func() {
		defer close(stopReturned)
		stopped <- store.Suppress(e.Admin(), consent.SuppressInput{
			ContactID: ids.From[ids.ContactKind](subject), Kind: commsauthz.ReasonSubjectRequest,
		})
	}()
	waitForStopLockWaiters(t, 2, stopReturned)
	if err := hold.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-anonymized; err != nil {
		t.Fatalf("anonymizing: %v", err)
	}
	if err := <-stopped; err != nil {
		t.Fatalf("recording the stop: %v", err)
	}

	returning := e.SeedContact(t, "Rosa Returns", &e.AdminUser)
	if _, err := owner.Exec(ctx, `
		INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'manual', 'human:x')`, returning, address); err != nil {
		t.Fatalf("seeding the returning address: %v", err)
	}
	blocked, err := consent.NewGate(store).InvitationSuppressed(e.Admin(), address)
	if err != nil {
		t.Fatalf("asking about the address: %v", err)
	}
	if !blocked {
		t.Fatal("the stop recorded during the anonymize does not reach the returning address")
	}
	var plain int
	if err := owner.QueryRow(ctx, `
		SELECT count(*) FROM communication_suppression
		 WHERE contact_id = $1 AND address_hash IS NULL AND revoked_at IS NULL`, subject).Scan(&plain); err != nil {
		t.Fatal(err)
	}
	if plain != 0 {
		t.Errorf("%d stop(s) landed on the anonymized record with no address to stop", plain)
	}
}
