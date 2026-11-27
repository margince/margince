// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

// customerLinkChecks are the finance checks an upgraded database holds
// `NOT VALID`, so rows written before them may name no customer link.
var customerLinkChecks = []string{"finance_invoice_names_its_customer_link", "finance_payment_names_its_customer_link"}

// WithCustomerLinkChecksLifted runs mutate with both customer link checks
// dropped, then adds each back from its own catalog definition. The rows it
// plants are what an upgraded database holds from before the checks.
func WithCustomerLinkChecksLifted(t *testing.T, mutate func(pgx.Tx) error) {
	t.Helper()
	withChecksLifted(t, customerLinkChecks, mutate)
}

// WithParticipantAddressCheckLifted runs mutate with the lowercase check on
// activity_participant.address dropped, then adds it back. The rows it plants
// hold an address as older writers stored it.
func WithParticipantAddressCheckLifted(t *testing.T, mutate func(pgx.Tx) error) {
	t.Helper()
	withChecksLifted(t, []string{"activity_participant_address_is_canonical"}, mutate)
}

// negativeCountFloorSQL names the floors an upgraded database holds `NOT
// VALID`, so rows written before them may hold a negative count. Read from the
// catalog, so a floor added later is lifted too.
const negativeCountFloorSQL = `SELECT conname FROM pg_constraint
	 WHERE contype = 'c' AND conname LIKE '%\_not\_negative'
	   AND conrelid IN ('activity'::regclass, 'ai_call'::regclass) ORDER BY conname`

// WithNegativeCountFloorsLifted runs mutate with every negative count floor
// dropped, then adds each back. The rows it plants hold a count as older
// writers stored it.
func WithNegativeCountFloorsLifted(t *testing.T, mutate func(pgx.Tx) error) {
	t.Helper()
	var floors []string
	withFixtureOwnerTx(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(t.Context(), negativeCountFloorSQL)
		if err != nil {
			return err
		}
		floors, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if !slices.Contains(floors, "activity_duration_not_negative") {
		t.Fatalf("the negative count floors read %v; the schema no longer holds the activity floor", floors)
	}
	withChecksLifted(t, floors, mutate)
}

// withChecksLifted runs mutate with the named checks dropped, then adds each
// back from its own catalog definition, in one owner transaction.
func withChecksLifted(t *testing.T, names []string, mutate func(pgx.Tx) error) {
	t.Helper()
	ctx := t.Context()
	withFixtureOwnerTx(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT conrelid::regclass::text, conname, pg_get_constraintdef(oid)
			  FROM pg_constraint WHERE conname = ANY($1) ORDER BY conname`, names)
		if err != nil {
			return err
		}
		checks, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Table, Name, Definition string }])
		if err != nil {
			return err
		}
		if len(checks) != len(names) {
			t.Fatalf("found %d of the checks %v; the schema no longer holds them", len(checks), names)
		}
		for _, check := range checks {
			if _, err := tx.Exec(ctx, `ALTER TABLE `+check.Table+` DROP CONSTRAINT `+pgx.Identifier{check.Name}.Sanitize()); err != nil {
				return err
			}
		}
		if err := mutate(tx); err != nil {
			return err
		}
		// A deferred key check that mutate queued blocks the ALTER below.
		if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
			return err
		}
		for _, check := range checks {
			if _, err := tx.Exec(ctx, `ALTER TABLE `+check.Table+` ADD CONSTRAINT `+pgx.Identifier{check.Name}.Sanitize()+` `+check.Definition); err != nil {
				return err
			}
		}
		return nil
	})
}
