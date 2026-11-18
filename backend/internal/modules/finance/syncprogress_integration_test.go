// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

// A pass that stops partway keeps what it wrote, and a pass whose own context
// ran out can still say that it failed.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ledgerRefusedFor answers every ledger but one, which fails as a vendor that
// stopped answering would.
type ledgerRefusedFor struct {
	directorySource
	refused string
}

func (s ledgerRefusedFor) InvoicesFor(ctx context.Context, customer string) (SourceLedger, error) {
	if customer == s.refused {
		return SourceLedger{}, errLedgerRefused
	}
	return s.directorySource.InvoicesFor(ctx, customer)
}

func (a *sourceAdmin) lastSuccessAt(t *testing.T) *time.Time {
	t.Helper()
	var at *time.Time
	if err := a.store.tx(a.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(a.ctx,
			`SELECT last_success_at FROM finance_connection WHERE id = $1`, a.connection).Scan(&at)
	}); err != nil {
		t.Fatalf("reading the connection's last success: %v", err)
	}
	return at
}

func TestAPassCutShortKeepsTheLedgersItAlreadyWrote(t *testing.T) {
	t.Parallel()
	a := setupSourceAdmin(t)
	a.syncDirectory(t)
	before := a.lastSuccessAt(t)
	if before == nil {
		t.Fatal("the first pass recorded no success")
	}
	beta := a.seedCompany(t, "Beta Corp", nil)
	ceto := a.seedCompany(t, "Ceto AG", nil)
	for external, company := range map[string]ids.CompanyID{"BETA-02": beta, "CETO-03": ceto} {
		if _, err := a.store.LinkCustomer(a.admin(), external, company); err != nil {
			t.Fatalf("linking %s: %v", external, err)
		}
	}

	// Links are read in customer order, so BETA-02 is written before CETO-03 fails.
	source := ledgerRefusedFor{customers: directory, refused: "CETO-03"}
	if _, err := a.store.SyncConnection(a.ctx, source); !errors.Is(err, errLedgerRefused) {
		t.Fatalf("the pass answered %v, want the refused ledger's failure", err)
	}
	if n := a.ownerScalarCount(t, `SELECT count(*) FROM finance_invoice WHERE company_id = $1`, beta); n == 0 {
		t.Error("the ledger read before the failure was not kept")
	}
	if n := a.ownerScalarCount(t, `SELECT count(*) FROM finance_invoice WHERE company_id = $1`, ceto); n != 0 {
		t.Errorf("%d invoices for the customer whose ledger was refused, want 0", n)
	}
	if after := a.lastSuccessAt(t); after == nil || !after.Equal(*before) {
		t.Errorf("last success moved from %v to %v on a pass that failed", before, after)
	}
}

func TestAFailureIsRecordedAfterThePassContextRanOut(t *testing.T) {
	t.Parallel()
	a := setupSourceAdmin(t)
	ended, cancel := context.WithCancel(a.ctx)
	cancel()

	cause := errors.New("the pass ran out of time")
	err := a.store.RecordSyncFailure(ended, cause)
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "recording the failure also failed") {
		t.Fatalf("RecordSyncFailure answered %v, want only the original cause", err)
	}
	if status := a.connectionStatus(t, a.connection); status != statusConnectionError {
		t.Errorf("connection status = %q after a failed pass, want %q", status, statusConnectionError)
	}
}
