// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The accounting source as an administrator manages it: connect, link the
// source's customers onto companies, relink and merge. Each case then checks
// where the next sync puts the money. The source is a test double at the
// accounting boundary; the store, the SQL and the merge are the real ones.

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const testBooks = "test_books"

// testBooksSource lists two customers and answers one invoice for each, in EUR
// unless currency names another.
type testBooksSource struct {
	checkErr error
	currency string
	netMinor int64
}

func (testBooksSource) Name() string                  { return testBooks }
func (s testBooksSource) Check(context.Context) error { return s.checkErr }
func (testBooksSource) Customers(context.Context) ([]finance.SourceCustomer, error) {
	return []finance.SourceCustomer{
		{ExternalID: "CUST-A", DisplayName: "Customer A"},
		{ExternalID: "CUST-B", DisplayName: "Customer B"},
	}, nil
}

// testBooksIssuedOn is the day every testBooks invoice is issued.
var testBooksIssuedOn = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func (s testBooksSource) InvoicesFor(_ context.Context, customer string) (finance.SourceLedger, error) {
	due := testBooksIssuedOn.AddDate(0, 0, 30)
	currency, net := "EUR", int64(10000)
	if s.currency != "" {
		currency, net = s.currency, s.netMinor
	}
	return finance.SourceLedger{Invoices: []finance.SourceInvoice{{
		ExternalID: customer + "-INV-1", Number: customer + "-1", IssuedOn: testBooksIssuedOn, DueOn: &due,
		Currency: currency, NetMinor: net, GrossMinor: net,
	}}}, nil
}

// testBooksSources offers testBooks, and nothing else.
type testBooksSources struct{ source testBooksSource }

func (s testBooksSources) Providers() []finance.SourceOption {
	return []finance.SourceOption{{Provider: testBooks, Description: "Test books."}}
}

func (s testBooksSources) Open(provider string, _ finance.SourceConnection) (finance.Provider, bool) {
	return s.source, provider == testBooks
}

func financeAdmin(e *Env) context.Context {
	objects := maps.Clone(AdminPerms.Objects)
	objects["integrations"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: AdminPerms.RoleKeys, Objects: objects, RowScope: principal.RowScopeAll,
	})
}

func financeStoreFor(e *Env, source testBooksSource) *finance.Store {
	return finance.NewStore(compose.InstallationDB(e.Pool), compose.FinanceRates()).
		WithSources(testBooksSources{source: source})
}

// connectAndSync connects testBooks and runs the first pass, which brings in
// its customer directory.
func connectAndSync(t *testing.T, e *Env, store *finance.Store) context.Context {
	t.Helper()
	admin := financeAdmin(e)
	if _, err := store.Connect(admin, testBooks); err != nil {
		t.Fatalf("connect: %v", err)
	}
	syncOnce(admin, t, store)
	return admin
}

func syncOnce(ctx context.Context, t *testing.T, store *finance.Store) {
	t.Helper()
	if _, err := store.SyncConnection(ctx, testBooksSource{}); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func invoicesOn(t *testing.T, e *Env, company ids.CompanyID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM finance_invoice WHERE company_id = $1`, company)
}

func TestConnectingAnAccountingSourceAsksItFirstAndAdmitsOne(t *testing.T) {
	e := Setup(t)
	admin := financeAdmin(e)

	unreadable := financeStoreFor(e, testBooksSource{checkErr: errors.New("bad credentials")})
	if _, err := unreadable.Connect(admin, testBooks); !errors.Is(err, apperrors.ErrProviderUnusable) {
		t.Fatalf("connect to an unreadable source = %v, want ErrProviderUnusable", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM finance_connection`); n != 0 {
		t.Fatalf("an unreadable source left %d connection rows for the sweep to fail on", n)
	}

	store := financeStoreFor(e, testBooksSource{})
	// A source this build does not compose is a fault of the provider field.
	_, err := store.Connect(admin, "not_installed")
	var refused *httperr.DetailedError
	if !errors.As(err, &refused) || refused.Code != "validation_error" ||
		len(refused.Fields) != 1 || refused.Fields[0].Field != "provider" {
		t.Errorf("connect to an uninstalled source = %v, want a validation error on provider", err)
	}
	first, err := store.Connect(admin, testBooks)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Connect(admin, testBooks)
	if err != nil || again.Connection == nil || again.Connection.Id != first.Connection.Id {
		t.Fatalf("connecting the connected source again = %+v, %v; want the same connection", again.Connection, err)
	}
	if err := store.Disconnect(admin); err != nil {
		t.Fatal(err)
	}
	state, err := store.SourceState(admin)
	if err != nil || state.Connection != nil {
		t.Fatalf("after a disconnect the state still names a connection: %+v, %v", state.Connection, err)
	}
}

// A relinked customer's invoices move to the new company on the next sync,
// although the source said nothing new about them.
func TestARelinkedCustomersInvoicesFollowTheLink(t *testing.T) {
	e := Setup(t)
	store := financeStoreFor(e, testBooksSource{})
	admin := connectAndSync(t, e, store)
	first, second := companyIDOf(e.SeedCompany(t, "Relink First", nil)), companyIDOf(e.SeedCompany(t, "Relink Second", nil))

	if _, err := store.LinkCustomer(admin, "CUST-A", first); err != nil {
		t.Fatal(err)
	}
	syncOnce(admin, t, store)
	if n := invoicesOn(t, e, first); n != 1 {
		t.Fatalf("the linked company holds %d invoices, want 1", n)
	}

	if _, err := store.LinkCustomer(admin, "CUST-A", second); err != nil {
		t.Fatal(err)
	}
	syncOnce(admin, t, store)
	if n := invoicesOn(t, e, first); n != 0 {
		t.Errorf("%d invoices stayed on the company the customer was linked away from", n)
	}
	if n := invoicesOn(t, e, second); n != 1 {
		t.Errorf("the newly linked company holds %d invoices, want 1", n)
	}

	page, _, err := store.ListSourceCustomers(admin, nil, nil)
	if err != nil || len(page) != 2 || page[0].Company == nil || ids.UUID(page[0].Company.Id) != second.UUID {
		t.Fatalf("directory = %+v, %v; want Customer A linked to the second company", page, err)
	}
	if err := store.UnlinkCustomer(admin, "CUST-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LinkCustomer(admin, "NOT-LISTED", second); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("linking a customer the source does not list = %v, want ErrNotFound", err)
	}
}

// Two companies each linked to an accounting customer, merged: both links
// land on the survivor and the next sync files every invoice there.
func TestAMergeOfTwoLinkedCompaniesKeepsBothLedgersOnTheSurvivor(t *testing.T) {
	e := Setup(t)
	store := financeStoreFor(e, testBooksSource{})
	admin := connectAndSync(t, e, store)
	src, tgt := seedMergePair(t, e, "Ledger merge")
	if _, err := store.LinkCustomer(admin, "CUST-A", src); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LinkCustomer(admin, "CUST-B", tgt); err != nil {
		t.Fatal(err)
	}
	syncOnce(admin, t, store)

	if _, err := e.Contacts.MergeCompany(admin, src, tgt, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	syncOnce(admin, t, store)

	if n := e.WsCount(t, `SELECT count(*) FROM finance_customer_link
		WHERE company_id = $1 AND archived_at IS NULL`, tgt); n != 2 {
		t.Errorf("the survivor holds %d live links, want both", n)
	}
	if n := invoicesOn(t, e, src); n != 0 {
		t.Errorf("%d invoices went back to the merged-away company", n)
	}
	if n := invoicesOn(t, e, tgt); n != 2 {
		t.Errorf("the survivor holds %d invoices, want both customers'", n)
	}
}
