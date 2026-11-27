// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func paymentExec(t *testing.T, a *sourceAdmin, statement string, args ...any) {
	t.Helper()
	positions := make([]any, len(args))
	for i := range args {
		positions[i] = i + 1
	}
	a.ownerExec(t, storekit.SQLf(statement, positions...), args...)
}

func paymentReader(a *sourceAdmin, user ids.UUID, scope principal.RowScope) context.Context {
	return a.asUser(user, scope, map[string]principal.ObjectGrant{"finance": {Read: true}, "company": {Read: true}})
}

func paymentResponse(ctx context.Context, t *testing.T, a *sourceAdmin, cursor *string, limit int) crmcontracts.CustomerPayments {
	t.Helper()
	response := call(ctx, http.MethodGet, "", func(w http.ResponseWriter, r *http.Request) {
		a.handlers.GetCustomerPayments(w, r, crmcontracts.GetCustomerPaymentsParams{Cursor: cursor, Limit: &limit})
	})
	expect(t, response, http.StatusOK, "")
	return decode[crmcontracts.CustomerPayments](t, response)
}

func syncPayments(t *testing.T, a *sourceAdmin) {
	t.Helper()
	a.store.SetClock(func() time.Time { return ledgerEpoch })
	if _, err := a.store.SyncConnection(a.ctx, a.provider()); err != nil {
		t.Fatal(err)
	}
}

func TestCustomerPaymentsNeverCountsAnUnsharedAccountAndRechecksEveryPage(t *testing.T) {
	a := setupSourceAdmin(t)
	reader, other := a.seedUser(t), a.seedUser(t)
	paymentExec(t, a, "UPDATE company SET owner_id = $%d WHERE id = $%d", reader, a.company)
	hidden := a.seedCompany(t, "Private customer's invoice", &other)
	paymentExec(t, a, `INSERT INTO finance_external_customer
  (connection_id, external_customer_id, display_name, sync_hash, source, captured_by)
  VALUES ($%d, 'HIDDEN', 'Hidden', '', 'manual', 'human:test')`, a.connection)
	paymentExec(t, a, `INSERT INTO finance_customer_link
  (connection_id, company_id, external_customer_id, captured_by)
  VALUES ($%d, $%d, 'HIDDEN', 'human:test')`, a.connection, hidden)
	syncPayments(t, a)
	ctx := paymentReader(a, reader, principal.RowScopeOwn)
	own := paymentResponse(ctx, t, a, nil, 1)
	if own.LinkedCompanies != 1 || own.CompanyCount != 1 || own.InvoiceCount != 1 || len(own.Totals) != 1 || own.Page.HasMore {
		t.Fatalf("hidden account changed the overview: %+v", own)
	}
	if len(own.Data) != 1 || ids.UUID(own.Data[0].CompanyId) != a.company.UUID {
		t.Fatalf("wrong account in invoice page: %+v", own.Data)
	}
	paymentExec(t, a, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
  VALUES ('company', $%d, 'user', $%d, 'read', $%d)`, hidden, reader, other)
	shared := paymentResponse(ctx, t, a, nil, 1)
	if shared.InvoiceCount != 2 || shared.LinkedCompanies != 2 || !shared.Page.HasMore ||
		shared.Totals[0].AmountMinor != own.Totals[0].AmountMinor*2 {
		t.Fatalf("live read share did not widen all figures together: %+v", shared)
	}
	paymentExec(t, a, `UPDATE record_grant SET created_at = '2000-01-01', expires_at = '2001-01-01' WHERE record_id = $%d`, hidden)
	revoked := paymentResponse(ctx, t, a, shared.Page.NextCursor, 1)
	if revoked.InvoiceCount != own.InvoiceCount || revoked.CompanyCount != own.CompanyCount ||
		revoked.Totals[0].AmountMinor != own.Totals[0].AmountMinor {
		t.Fatalf("expired share still affects totals: %+v", revoked)
	}
	if shared.AccessScope == own.AccessScope || revoked.AccessScope != own.AccessScope {
		t.Fatal("page revisions did not track the readable account set")
	}
	raw, err := json.Marshal(revoked)
	if err != nil {
		t.Fatal(err)
	}
	for _, customer := range revoked.Customers {
		if ids.UUID(customer.CompanyId) == hidden.UUID {
			t.Fatalf("hidden preview returned: %s", raw)
		}
	}
	for _, invoice := range revoked.Data {
		if ids.UUID(invoice.CompanyId) == hidden.UUID {
			t.Fatalf("hidden invoice returned: %s", raw)
		}
	}
}

func TestCustomerPaymentsKeepsCapturePrivateAccountsOutOfAllScope(t *testing.T) {
	a := setupSourceAdmin(t)
	a.makeOwnerPrivate(t)
	syncPayments(t, a)
	got := paymentResponse(a.documentReader(principal.RowScopeAll), t, a, nil, 50)
	if got.LinkedCompanies != 0 || got.InvoiceCount != 0 || len(got.Totals) != 0 || len(got.Data) != 0 || len(got.Customers) != 0 {
		t.Fatalf("capture-private account escaped through aggregates: %+v", got)
	}
}

func TestCustomerPaymentsRequiresBothReadGrants(t *testing.T) {
	a := setupSourceAdmin(t)
	for _, missing := range []string{"finance", "company"} {
		grants := map[string]principal.ObjectGrant{"finance": {Read: true}, "company": {Read: true}}
		delete(grants, missing)
		response := call(a.as(principal.RowScopeAll, grants), http.MethodGet, "", func(w http.ResponseWriter, r *http.Request) {
			a.handlers.GetCustomerPayments(w, r, crmcontracts.GetCustomerPaymentsParams{})
		})
		expect(t, response, http.StatusForbidden, "permission_denied")
	}
}

func TestCustomerPaymentsDoesNotCallAnIncompleteFirstSyncAZeroBalance(t *testing.T) {
	a := setupSourceAdmin(t)
	syncPayments(t, a)
	paymentExec(t, a, `UPDATE finance_connection SET status = 'error', last_success_at = NULL WHERE id = $%d`, a.connection)
	got := paymentResponse(a.documentReader(principal.RowScopeAll), t, a, nil, 50)
	if got.State != crmcontracts.FinanceSummaryStateError || got.LastSyncedAt != nil ||
		got.InvoiceCount != 0 || len(got.Totals) != 0 || len(got.Data) != 0 {
		t.Fatalf("partial first sync became a complete money claim: %+v", got)
	}
}

func TestCustomerPaymentsLeavesArchivedAndUnlinkedAndFormerCustomersOut(t *testing.T) {
	for _, state := range []string{"archived", "unlinked", "former"} {
		t.Run(state, func(t *testing.T) {
			a := setupSourceAdmin(t)
			syncPayments(t, a)
			before := paymentResponse(a.documentReader(principal.RowScopeAll), t, a, nil, 50)
			if before.InvoiceCount == 0 {
				t.Fatal("fixture has no overdue invoice")
			}
			switch state {
			case "archived":
				paymentExec(t, a, `UPDATE company SET archived_at = now() WHERE id = $%d`, a.company)
			case "unlinked":
				paymentExec(t, a, `UPDATE finance_customer_link SET archived_at = now() WHERE company_id = $%d`, a.company)
			case "former":
				a.setLifecycle(t, lifecycleFormerCustomer)
			}
			got := paymentResponse(a.documentReader(principal.RowScopeAll), t, a, nil, 50)
			if got.LinkedCompanies != 0 || got.InvoiceCount != 0 || len(got.Totals) != 0 {
				t.Fatalf("ineligible account returned: %+v", got)
			}
		})
	}
}

func TestCustomerPaymentsKeepsCompleteTotalsAcrossEveryInvoicePage(t *testing.T) {
	a := setupSourceAdmin(t)
	a.store.SetClock(func() time.Time { return ledgerEpoch.AddDate(0, 0, 60) })
	if _, err := a.store.SyncConnection(a.ctx, a.provider()); err != nil {
		t.Fatal(err)
	}
	ctx := a.documentReader(principal.RowScopeAll)
	first := paymentResponse(ctx, t, a, nil, 1)
	if first.InvoiceCount < 2 || !first.Page.HasMore {
		t.Fatalf("fixture has no second overdue invoice: %+v", first)
	}
	seen := map[string]bool{}
	page := first
	for {
		if page.InvoiceCount != first.InvoiceCount || page.Totals[0] != first.Totals[0] || page.AccessScope != first.AccessScope {
			t.Fatal("page changed the overview")
		}
		for _, invoice := range page.Data {
			id := invoice.InvoiceId.String()
			if seen[id] {
				t.Fatalf("invoice %s repeated across pages", id)
			}
			seen[id] = true
		}
		if !page.Page.HasMore {
			break
		}
		page = paymentResponse(ctx, t, a, page.Page.NextCursor, 1)
	}
	if len(seen) != first.InvoiceCount {
		t.Fatalf("paged %d of %d overdue invoices", len(seen), first.InvoiceCount)
	}
}

func TestCustomerPaymentsDoesNotIncludeTheReplacedConnectionsInvoices(t *testing.T) {
	a := setupSourceAdmin(t)
	syncPayments(t, a)
	replacement := ids.NewV7()
	paymentExec(t, a, `INSERT INTO finance_connection (id,provider,status,captured_by,created_at)
  VALUES ($%d,$%d,'active','human:test',now()+interval '1 second')`, replacement, fixtureProviderName)
	// The package shares one database, and every later test reads the newest connection.
	t.Cleanup(func() {
		a.ownerExec(t, `UPDATE finance_connection SET status = 'disconnected' WHERE id = $1`, replacement)
	})
	got := paymentResponse(a.documentReader(principal.RowScopeAll), t, a, nil, 50)
	if got.State != crmcontracts.FinanceSummaryStateSyncing || got.LinkedCompanies != 0 || got.InvoiceCount != 0 || len(got.Data) != 0 {
		t.Fatalf("old source figures appeared: %+v", got)
	}
}

func TestCustomerPaymentsRejectsMalformedAndWrongSourceCursors(t *testing.T) {
	a := setupSourceAdmin(t)
	syncPayments(t, a)
	wrong, err := storekit.EncodeOpaque(paymentPosition{ids.NewV7(), ledgerEpoch, ids.NewV7()})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"broken", "e30", wrong} {
		response := call(a.documentReader(principal.RowScopeAll), http.MethodGet, "", func(w http.ResponseWriter, r *http.Request) {
			a.handlers.GetCustomerPayments(w, r, crmcontracts.GetCustomerPaymentsParams{Cursor: &token})
		})
		expect(t, response, http.StatusUnprocessableEntity, "validation_error")
	}
}

func TestCustomerPaymentsWaitsForANewCustomerLinksFirstImport(t *testing.T) {
	a := setupSourceAdmin(t)
	a.syncDirectory(t)
	another := a.seedCompany(t, "Newly linked account", nil)
	if _, err := a.store.LinkCustomer(a.admin(), "BETA-02", another); err != nil {
		t.Fatal(err)
	}
	got := paymentResponse(a.documentReader(principal.RowScopeAll), t, a, nil, 50)
	if got.State != crmcontracts.FinanceSummaryStateSyncing || got.LinkedCompanies != 2 || len(got.Totals) != 0 || len(got.Data) != 0 || len(got.Customers) != 0 {
		t.Fatalf("new link was reported as a complete ledger: %+v", got)
	}
	a.syncDirectory(t)
	got = paymentResponse(a.documentReader(principal.RowScopeAll), t, a, nil, 50)
	if got.State != crmcontracts.FinanceSummaryStateConnected || got.LinkedCompanies != 2 || got.CompanyCount != 2 || got.InvoiceCount == 0 {
		t.Fatalf("completed import did not restore the overview: %+v", got)
	}
}

func TestCustomerPaymentsKeepsLastGoodFiguresWhenANewLinkFollowsAFailedSync(t *testing.T) {
	a := setupSourceAdmin(t)
	a.syncDirectory(t)
	ctx := a.documentReader(principal.RowScopeAll)
	before := paymentResponse(ctx, t, a, nil, 50)
	another := a.seedCompany(t, "Waiting account", nil)
	if _, err := a.store.LinkCustomer(a.admin(), "BETA-02", another); err != nil {
		t.Fatal(err)
	}
	paymentExec(t, a, "UPDATE finance_connection SET status = 'error' WHERE id = $%d", a.connection)
	got := paymentResponse(ctx, t, a, nil, 50)
	if got.State != crmcontracts.FinanceSummaryStateError || got.InvoiceCount != before.InvoiceCount || len(got.Totals) != len(before.Totals) || got.Totals[0] != before.Totals[0] {
		t.Fatalf("new link erased failed-sync last-good figures: %+v", got)
	}
}
