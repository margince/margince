// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

// A row mirrored before rows named their customer link may still name none.
// The link checks are `NOT VALID`, so such a row stays, and every new or
// rewritten row must name a link.

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// beforeTheLinkCheck runs plant as the owner with the customer link checks
// lifted, then restores each from its own catalog definition. That leaves the
// rows as an upgraded database holds rows mirrored before the checks.
func (e *financeEnv) beforeTheLinkCheck(t *testing.T, plant func(tx pgx.Tx) error) {
	t.Helper()
	owner, err := pgx.Connect(e.ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("owner connection: %v", err)
	}
	defer func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	err = pgx.BeginFunc(e.ctx, owner, func(tx pgx.Tx) error {
		rows, err := tx.Query(e.ctx, `SELECT conrelid::regclass::text, conname, pg_get_constraintdef(oid) FROM pg_constraint
			 WHERE conrelid IN ('finance_invoice'::regclass, 'finance_payment'::regclass)
			   AND conname LIKE '%\_names\_its\_customer\_link'`)
		if err != nil {
			return err
		}
		checks, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Table, Name, Definition string }])
		if err != nil {
			return err
		}
		for _, check := range checks {
			if _, err := tx.Exec(e.ctx, `ALTER TABLE `+check.Table+` DROP CONSTRAINT `+check.Name); err != nil {
				return err
			}
		}
		if err := plant(tx); err != nil {
			return err
		}
		// A deferred key check queued by plant blocks the ALTER below.
		if _, err := tx.Exec(e.ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
			return err
		}
		for _, check := range checks {
			if _, err := tx.Exec(e.ctx, `ALTER TABLE `+check.Table+` ADD CONSTRAINT `+check.Name+` `+check.Definition); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("planting rows mirrored before the link checks: %v", err)
	}
}

// unlinkTheFixtureRows clears the link on every row of the fixture connection.
func (e *financeEnv) unlinkTheFixtureRows(tx pgx.Tx) error {
	for _, table := range []string{"finance_invoice", "finance_payment"} {
		if _, err := tx.Exec(e.ctx, `UPDATE `+table+` SET customer_link_id = NULL WHERE connection_id = $1`, e.connection); err != nil {
			return err
		}
	}
	return nil
}

// A new row, and a rewrite of an old row, must name a customer link. An old
// row that names none stays as it is.
func TestTheDatabaseRefusesANewOrRewrittenRowThatNamesNoCustomerLink(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"finance_invoice", "finance_payment"} {
		want := table + "_names_its_customer_link"
		if got := e.ownerRefused(t, `UPDATE `+table+` SET customer_link_id = NULL WHERE connection_id = $1`, e.connection); got != want {
			t.Errorf("unlinking a %s row was refused by %q, want %q", table, got, want)
		}
	}
	unmapped := ids.NewV7()
	if got := e.ownerRefused(t, `INSERT INTO finance_invoice (id, connection_id, company_id, external_id, issued_at, status,
		currency, net_minor, gross_minor, sync_hash, source, captured_by)
		VALUES ($1, $2, $3, 'NO-LINK', current_date, 'open', 'EUR', 1000, 1190, 'hash', 'test', 'system:test')`,
		unmapped, e.connection, e.company); got != "finance_invoice_names_its_customer_link" {
		t.Errorf("a new invoice naming no link was refused by %q, want the link check", got)
	}

	e.beforeTheLinkCheck(t, e.unlinkTheFixtureRows)
	if got := e.ownerRefused(t, `UPDATE finance_invoice SET number = number WHERE connection_id = $1`, e.connection); got != "finance_invoice_names_its_customer_link" {
		t.Errorf("a rewrite of an old invoice naming no link was refused by %q, want the link check", got)
	}
	if left := e.ownerScalarCount(t, `SELECT count(*) FROM finance_invoice WHERE connection_id = $1 AND customer_link_id IS NULL`, e.connection); left == 0 {
		t.Error("the old rows naming no link are gone; the check must leave them as they are")
	}
}
