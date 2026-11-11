// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

// Money in the mirror sits only on a company its connection mapped. The sync
// writes every row through its customer link, and the database refuses a row
// that names any other company.

import (
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// linkOf reads the id of the fixture customer's live link.
func (e *financeEnv) linkOf(t *testing.T) ids.UUID {
	t.Helper()
	return e.ownerScalar(t, `SELECT id FROM finance_customer_link
		 WHERE connection_id = $1 AND external_customer_id = $2 AND archived_at IS NULL`, e.connection, e.external)
}

func (e *financeEnv) ownerScalar(t *testing.T, query string, args ...any) ids.UUID {
	t.Helper()
	owner, err := pgx.Connect(e.ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("owner connection: %v", err)
	}
	defer func() {
		if err := owner.Close(e.ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	var id ids.UUID
	if err := owner.QueryRow(e.ctx, query, args...).Scan(&id); err != nil {
		t.Fatalf("owner read %q: %v", query, err)
	}
	return id
}

// ownerRefused runs one statement as the owner and answers the constraint
// that refused it, or "" when it committed.
func (e *financeEnv) ownerRefused(t *testing.T, statement string, args ...any) string {
	t.Helper()
	owner, err := pgx.Connect(e.ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("owner connection: %v", err)
	}
	defer func() {
		if err := owner.Close(e.ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	_, err = owner.Exec(e.ctx, statement, args...)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	if err != nil {
		t.Fatalf("owner statement %q: %v", statement, err)
	}
	return ""
}

// otherCompany seeds a second company with no link on the fixture connection.
func (e *financeEnv) otherCompany(t *testing.T) ids.UUID {
	t.Helper()
	other := ids.NewV7()
	e.ownerExec(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Elsewhere AG', 'manual', 'human:test')`, other)
	return other
}

func TestTheSyncWritesEveryMirroredRowThroughItsCustomerLink(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	link := e.linkOf(t)
	for _, table := range []string{"finance_invoice", "finance_payment"} {
		stray := e.ownerScalarCount(t, `SELECT count(*) FROM `+table+`
			 WHERE connection_id = $1 AND customer_link_id IS DISTINCT FROM $2`, e.connection, link)
		if stray != 0 {
			t.Errorf("%d %s rows name no customer link or another one", stray, table)
		}
	}
}

func (e *financeEnv) ownerScalarCount(t *testing.T, query string, args ...any) int {
	t.Helper()
	owner, err := pgx.Connect(e.ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("owner connection: %v", err)
	}
	defer func() {
		if err := owner.Close(e.ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	var n int
	if err := owner.QueryRow(e.ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("owner count %q: %v", query, err)
	}
	return n
}

// The database refuses each way a mirrored row could land on a company its
// connection never mapped, and a credit note that is not one.
func TestTheDatabaseRefusesAMisattributedRow(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	link := e.linkOf(t)
	other := e.otherCompany(t)
	// An invoice nothing points at, so only its own link key can refuse the move.
	invoice := e.ownerScalar(t, `SELECT i.id FROM finance_invoice i
		 WHERE i.connection_id = $1 AND i.credits_invoice_id IS NULL
		   AND NOT EXISTS (SELECT 1 FROM finance_invoice n WHERE n.credits_invoice_id = i.id)
		   AND NOT EXISTS (SELECT 1 FROM finance_payment p WHERE p.invoice_id = i.id)
		 ORDER BY i.id LIMIT 1`, e.connection)
	for _, test := range []struct {
		name, statement, want string
		args                  []any
	}{
		{"an invoice on a company its link does not name", `UPDATE finance_invoice SET company_id = $2 WHERE id = $1`,
			"finance_invoice_customer_link_fk", []any{invoice, other}},
		{"a payment on a company its link does not name", `UPDATE finance_payment SET company_id = $2, invoice_id = NULL WHERE connection_id = $1`,
			"finance_payment_customer_link_fk", []any{e.connection, other}},
		{"a link to a customer the directory does not list", `INSERT INTO finance_customer_link
			(connection_id, company_id, external_customer_id, sync_hash, source, captured_by)
			VALUES ($1, $2, 'NEVER-LISTED', '', 'manual', 'human:test')`,
			"finance_customer_link_directory_fk", []any{e.connection, other}},
		{"a credit note that is not credited", `UPDATE finance_invoice SET credits_invoice_id = $2, status = 'open'
			WHERE id = (SELECT id FROM finance_invoice WHERE connection_id = $1 AND id <> $2 AND credits_invoice_id IS NULL ORDER BY id LIMIT 1)`,
			"finance_invoice_credit_note_status", []any{e.connection, invoice}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := e.ownerRefused(t, test.statement, test.args...); got != test.want {
				t.Errorf("refused by %q, want %q", got, test.want)
			}
		})
	}
	t.Run("a payment on another customer's invoice", func(t *testing.T) {
		foreign := e.foreignInvoice(t, other)
		if got := e.ownerRefused(t, `UPDATE finance_payment SET invoice_id = $2 WHERE connection_id = $1`, e.connection, foreign); got != "finance_payment_invoice_owner_fk" {
			t.Errorf("refused by %q, want finance_payment_invoice_owner_fk", got)
		}
	})
	t.Run("a credit note on another customer's invoice", func(t *testing.T) {
		foreign := e.foreignInvoice(t, other)
		got := e.ownerRefused(t, `UPDATE finance_invoice SET credits_invoice_id = $2, status = 'credited'
			WHERE id = (SELECT id FROM finance_invoice WHERE connection_id = $1 AND company_id <> $3 ORDER BY id LIMIT 1)`,
			e.connection, foreign, other)
		if got != "finance_invoice_credits_owner_fk" {
			t.Errorf("refused by %q, want finance_invoice_credits_owner_fk", got)
		}
	})
	if after := e.linkOf(t); after != link {
		t.Errorf("the live link moved from %s to %s during refused writes", link, after)
	}
}

// foreignInvoice seeds a second customer mapped to another company on the same
// connection, with one invoice of its own.
func (e *financeEnv) foreignInvoice(t *testing.T, company ids.UUID) ids.UUID {
	t.Helper()
	external := "OTHER-" + ids.NewV7().String()
	e.ownerExec(t, `INSERT INTO finance_external_customer (connection_id, external_customer_id, display_name, sync_hash, source, captured_by)
		VALUES ($1, $2, 'Other customer', '', 'system', 'system:test')`, e.connection, external)
	link := ids.NewV7()
	e.ownerExec(t, `INSERT INTO finance_customer_link (id, connection_id, company_id, external_customer_id, sync_hash, source, captured_by)
		VALUES ($1, $2, $3, $4, '', 'manual', 'human:test')`, link, e.connection, company, external)
	invoice := ids.NewV7()
	e.ownerExec(t, `INSERT INTO finance_invoice (id, connection_id, company_id, customer_link_id, external_id, issued_at, status,
		currency, net_minor, gross_minor, sync_hash, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, current_date, 'open', 'EUR', 1000, 1190, 'hash', 'test', 'system:test')`,
		invoice, e.connection, company, link, "INV-"+external)
	return invoice
}

// A row mirrored before rows named their link gets its link on the next pass.
func TestASyncNamesTheLinkOnARowMirroredBeforeRowsNamedOne(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	e.ownerExec(t, `UPDATE finance_invoice SET customer_link_id = NULL WHERE connection_id = $1`, e.connection)
	e.ownerExec(t, `UPDATE finance_payment SET customer_link_id = NULL WHERE connection_id = $1`, e.connection)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"finance_invoice", "finance_payment"} {
		if unnamed := e.ownerScalarCount(t, `SELECT count(*) FROM `+table+` WHERE connection_id = $1 AND customer_link_id IS NULL`, e.connection); unnamed != 0 {
			t.Errorf("%d %s rows still name no link after a pass", unnamed, table)
		}
	}
}

// The backfill names the archived link on rows a sync no longer reads, and
// leaves NULL on a row whose connection never mapped its company.
func TestTheBackfillNamesTheArchivedLinkAndLeavesAnUnmappedRow(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatal(err)
	}
	link := e.linkOf(t)
	e.ownerExec(t, `UPDATE finance_invoice SET customer_link_id = NULL WHERE connection_id = $1`, e.connection)
	e.ownerExec(t, `UPDATE finance_payment SET customer_link_id = NULL WHERE connection_id = $1`, e.connection)
	e.ownerExec(t, `UPDATE finance_customer_link SET archived_at = now() WHERE id = $1`, link)
	unmapped := ids.NewV7()
	other := e.otherCompany(t)
	e.ownerExec(t, `INSERT INTO finance_invoice (id, connection_id, company_id, external_id, issued_at, status,
		currency, net_minor, gross_minor, sync_hash, source, captured_by)
		VALUES ($1, $2, $3, 'NEVER-MAPPED', current_date, 'open', 'EUR', 1000, 1190, 'hash', 'test', 'system:test')`,
		unmapped, e.connection, other)

	ctx := principal.SystemActing(e.ctx, "system:datatask-test")
	for _, table := range []MirrorTable{MirrorInvoices, MirrorPayments} {
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, _, err := BackfillCustomerLinksTx(ctx, tx, table, ids.Nil, 10000)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"finance_invoice", "finance_payment"} {
		stray := e.ownerScalarCount(t, `SELECT count(*) FROM `+table+`
			 WHERE connection_id = $1 AND company_id = $2 AND customer_link_id IS DISTINCT FROM $3`, e.connection, e.company, link)
		if stray != 0 {
			t.Errorf("%d %s rows of the unlinked customer name no link or another one", stray, table)
		}
	}
	if left := e.ownerScalarCount(t, `SELECT count(*) FROM finance_invoice WHERE id = $1 AND customer_link_id IS NULL`, unmapped); left != 1 {
		t.Error("the backfill named a link on a row whose company the connection never mapped")
	}
	audits := e.ownerScalarCount(t, `SELECT count(*) FROM audit_log WHERE action = 'update' AND actor_id = 'system:datatask-test'
		AND after ? 'customer_link_id'`)
	if audits == 0 {
		t.Error("the backfill wrote no audit row for the links it named")
	}
}
