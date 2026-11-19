// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
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
	ctx := t.Context()
	withFixtureOwnerTx(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT conrelid::regclass::text, conname, pg_get_constraintdef(oid)
			  FROM pg_constraint WHERE conname = ANY($1) ORDER BY conname`, customerLinkChecks)
		if err != nil {
			return err
		}
		checks, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Table, Name, Definition string }])
		if err != nil {
			return err
		}
		if len(checks) != len(customerLinkChecks) {
			t.Fatalf("found %d of the %d customer link checks; the schema no longer holds them", len(checks), len(customerLinkChecks))
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
