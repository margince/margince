// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheNightlyScanReopensAPhonePairAfterANumberIsRevived(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedContact(ctx, t, "Ola Berg", []string{"ola@phone.example"}, []string{"+49305550101"})
	right := e.seedContact(ctx, t, "Kim Lund", []string{"kim@phone.example"}, []string{"+49305550101"})
	var phoneID ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM contact_phone WHERE contact_id = @id`,
			pgx.StrictNamedArgs{"id": right}).Scan(&phoneID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateContact(ctx, right, UpdateContactInput{Phones: []ContactPhoneInput{}}); err != nil {
		t.Fatal(err)
	}
	if got := e.recheck(t); got.Closed != 1 {
		t.Fatalf("removed number kept its pair: %+v", got)
	}
	ageAllScannedRows(t)
	e.scanChanged(t)
	ageAllScannedRows(t)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		revived, err := revivePhoneTx(ctx, tx, right, phoneID)
		if err == nil && !revived {
			t.Error("the number was not revived")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.scanChanged(t); got.Filed != 1 {
		t.Fatalf("the phone's own timestamp did not reopen its pair: %+v", got)
	}
	queue, _, err := e.store.ListDedupeCandidates(ctx, DedupeQueueInput{EntityType: entityContact})
	if err != nil || len(queue) != 1 {
		t.Fatalf("the revived number has no open pair: %+v, error %v", queue, err)
	}
}
