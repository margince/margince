// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestConsentVersionRepairWaitsForAnUncommittedWithdrawal(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.store.Record(e.ctx, RecordInput{ContactID: e.contact, PurposeID: e.newsletter, NewState: "granted", PolicyText: &grantWording}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(e.ctx, `UPDATE contact_consent SET policy_version = NULL WHERE contact_id = $1`, e.contact); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
	defer cancel()
	in := RecordInput{ContactID: e.contact, PurposeID: e.newsletter, NewState: "withdrawn"}
	sub, state, err := admitRecord(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	written, release := make(chan uint32, 1), make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	defer finish()
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- e.store.db.Tx(ctx, func(tx pgx.Tx) error {
			if _, err := e.store.recordAdmittedTx(ctx, tx, in, sub, state); err != nil {
				return err
			}
			written <- tx.Conn().PgConn().PID()
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var writerPID uint32
	select {
	case writerPID = <-written:
	case err := <-writerDone:
		t.Fatalf("withdrawal before the repair: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	repairDone := make(chan error, 1)
	repairReturned := make(chan struct{})
	system := principal.SystemActing(ctx, "system:consent-version-test")
	go func() {
		defer close(repairReturned)
		repairDone <- e.store.db.Tx(system, func(tx pgx.Tx) error {
			_, err := BackfillConsentVersionsTx(system, tx, ids.Nil, 100)
			return err
		})
	}()
	testdb.AwaitQueuedBehind(t, e.owner, int32(writerPID), 1, repairReturned, "consent version repair")
	finish()
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-repairDone; err != nil {
		t.Fatal(err)
	}
	text, version := wordingOf(t, e)
	if text != nil || version != nil {
		t.Fatalf("repair attached an old grant version to the withdrawal: %v / %v", text, version)
	}
}

func TestConsentVersionRepairRequiresASystemActor(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := BackfillConsentVersionsTx(e.ctx, nil, ids.Nil, 1); err == nil {
		t.Fatal("human caller reached the repair")
	}
}
