// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A foreign invoice in the finance mirror, converted through the wiring the
// product runs. The deals rate sheet picks the rate in force on the issue
// date, and the one conversion applies both currencies' minor-unit scales.

import (
	"context"
	"maps"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// financeFxNow is after every rate and invoice date these cases use. So each
// rate they enter is a past date, which the sheet accepts only as history.
var financeFxNow = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// linkedForeignLedger connects testBooks billing in currency, links its first
// customer to a new company, and answers the store, the admin and the company.
func linkedForeignLedger(
	t *testing.T, e *Env, source testBooksSource,
) (*finance.Store, context.Context, ids.CompanyID) {
	t.Helper()
	e.Deals.WithClock(func() time.Time { return financeFxNow })
	store := financeStoreFor(e, source).WithClock(func() time.Time { return financeFxNow })
	admin := financeAdmin(e)
	if _, err := store.Connect(admin, testBooks); err != nil {
		t.Fatal(err)
	}
	mirrorPass(admin, t, store, source)
	company := companyIDOf(e.SeedCompany(t, "Foreign ledger "+source.currency, nil))
	if _, err := store.LinkCustomer(admin, "CUST-A", company); err != nil {
		t.Fatal(err)
	}
	return store, admin, company
}

func mirrorPass(ctx context.Context, t *testing.T, store *finance.Store, source testBooksSource) finance.SyncResult {
	t.Helper()
	result, err := store.SyncConnection(ctx, source)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	return result
}

func setFinanceRate(ctx context.Context, t *testing.T, e *Env, currency, rate string, day time.Time) {
	t.Helper()
	if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{
		FromCurrency: currency, Rate: rate, EffectiveDate: day,
	}); err != nil {
		t.Fatalf("set %s at %s on %s: %v", currency, rate, day.Format(time.DateOnly), err)
	}
}

// frozenRate is the rate and date the mirror stored on the company's invoice,
// or empty strings when it stored none.
func frozenRate(t *testing.T, e *Env, company ids.CompanyID) (rate, day string) {
	t.Helper()
	return e.WsScalar(t, `SELECT coalesce(fx_rate_to_base::text, '') FROM finance_invoice WHERE company_id = $1`, company),
		e.WsScalar(t, `SELECT coalesce(fx_rate_date::text, '') FROM finance_invoice WHERE company_id = $1`, company)
}

// lifetimeNet reads the finance card's lifetime figure as a reader holding the
// finance grant the card asks for.
func lifetimeNet(t *testing.T, e *Env, store *finance.Store, company ids.CompanyID) *int64 {
	t.Helper()
	objects := maps.Clone(AdminPerms.Objects)
	objects["finance"] = principal.ObjectGrant{Read: true}
	reader := e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: AdminPerms.RoleKeys, Objects: objects, RowScope: principal.RowScopeAll,
	})
	summary, err := store.SummaryFor(reader, company)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.NetInvoicedLifetime == nil {
		return nil
	}
	return summary.NetInvoicedLifetime.AmountMinor
}

// amountOf spells a figure the card may have withheld.
func amountOf(minor *int64) string {
	if minor == nil {
		return "withheld"
	}
	return strconv.FormatInt(*minor, 10)
}

// A yen invoice against a euro base. Yen carry no minor unit and euros carry
// two, so a conversion that skipped either scale is off by a factor of 100.
// The rate dated after the issue day is in the sheet too, and must not be used.
func TestAForeignInvoiceConvertsAtTheRateInForceOnItsIssueDate(t *testing.T) {
	e := Setup(t)
	source := testBooksSource{currency: "JPY", netMinor: 1_000_000}
	store, admin, company := linkedForeignLedger(t, e, source)
	setFinanceRate(admin, t, e, "JPY", "0.0070", testBooksIssuedOn.AddDate(0, 0, 14))
	setFinanceRate(admin, t, e, "JPY", "0.0060", testBooksIssuedOn.AddDate(0, -1, 0))

	mirrorPass(admin, t, store, source)

	rate, day := frozenRate(t, e, company)
	if rate != "0.0060000000" || day != "2026-05-01" {
		t.Fatalf("froze %q on %q, want 0.0060000000 on 2026-05-01, the rate in force on the issue date", rate, day)
	}
	// ¥1,000,000 × 0.006 = €6,000.00, which is 600000 euro cents.
	if net := lifetimeNet(t, e, store, company); net == nil || *net != 600_000 {
		t.Fatalf("lifetime net = %s, want 600000 (€6,000.00)", amountOf(net))
	}
}

// A foreign invoice mirrored before its day had a rate stays unconverted and
// refuses the total. Once a rate is entered for that day, the next pass
// converts it, and the pass after that writes nothing.
func TestAForeignInvoiceConvertsOnceItsDayHasARate(t *testing.T) {
	e := Setup(t)
	source := testBooksSource{currency: "CHF", netMinor: 10_000}
	store, admin, company := linkedForeignLedger(t, e, source)

	mirrorPass(admin, t, store, source)
	if rate, day := frozenRate(t, e, company); rate != "" || day != "" {
		t.Fatalf("froze %q on %q with no CHF rate in the sheet, want neither", rate, day)
	}
	if net := lifetimeNet(t, e, store, company); net != nil {
		t.Fatalf("lifetime net = %d with an unconverted invoice, want no figure", *net)
	}
	if again := mirrorPass(admin, t, store, source); again.InvoicesUpdate != 0 {
		t.Fatalf("a pass with still no rate rewrote %d invoices, want none", again.InvoicesUpdate)
	}

	setFinanceRate(admin, t, e, "CHF", "1.05", testBooksIssuedOn.AddDate(0, 0, -7))
	if repair := mirrorPass(admin, t, store, source); repair.InvoicesUpdate != 1 {
		t.Fatalf("the pass after the rate arrived rewrote %d invoices, want 1", repair.InvoicesUpdate)
	}
	if rate, day := frozenRate(t, e, company); rate != "1.0500000000" || day != "2026-05-25" {
		t.Fatalf("froze %q on %q, want 1.0500000000 on 2026-05-25", rate, day)
	}
	if net := lifetimeNet(t, e, store, company); net == nil || *net != 10_500 {
		t.Fatalf("lifetime net = %s, want 10500", amountOf(net))
	}
	if settled := mirrorPass(admin, t, store, source); settled.InvoicesUpdate != 0 {
		t.Fatalf("the pass after the repair rewrote %d invoices, want none", settled.InvoicesUpdate)
	}
}
