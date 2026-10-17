// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

// The source's customer directory over HTTP, and the links an administrator
// makes from it onto companies. It covers paging and who sees which company.
// It also covers the write authority a link needs on its company.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// directory is the source's customer list, in the order the page answers it.
var directory = []SourceCustomer{
	{ExternalID: "ACME-01", DisplayName: "Acme Ltd"},
	{ExternalID: "BETA-02", DisplayName: "Beta Corp"},
	{ExternalID: "CETO-03", DisplayName: "Ceto AG"},
}

// syncDirectory mirrors the directory through the sweep's own writer.
func (a *sourceAdmin) syncDirectory(t *testing.T) {
	t.Helper()
	if _, err := a.store.SyncConnection(a.ctx, directorySource{customers: directory}); err != nil {
		t.Fatalf("mirroring the directory: %v", err)
	}
}

func (a *sourceAdmin) seedCompany(t *testing.T, name string, owner *ids.UUID) ids.CompanyID {
	t.Helper()
	id := ids.New[ids.CompanyKind]()
	a.ownerExec(t, `INSERT INTO company (id, display_name, lifecycle, source, captured_by, owner_id)
		VALUES ($1, $2, 'customer', 'manual', 'human:test', $3)`, id, name, owner)
	return id
}

func (a *sourceAdmin) seedUser(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	a.ownerExec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Stranger')`,
		id, id.String()+"@finance.test")
	return id
}

func (a *sourceAdmin) list(ctx context.Context, cursor *string, limit int) *httptest.ResponseRecorder {
	return call(ctx, http.MethodGet, "", func(w http.ResponseWriter, r *http.Request) {
		a.handlers.ListFinanceSourceCustomers(w, r,
			crmcontracts.ListFinanceSourceCustomersParams{Cursor: cursor, Limit: &limit})
	})
}

func (a *sourceAdmin) entries(ctx context.Context, t *testing.T) map[string]crmcontracts.FinanceSourceCustomer {
	t.Helper()
	rec := a.list(ctx, nil, 50)
	expect(t, rec, http.StatusOK, "")
	out := map[string]crmcontracts.FinanceSourceCustomer{}
	for _, each := range decode[crmcontracts.FinanceSourceCustomerListResponse](t, rec).Data {
		out[each.ExternalId] = each
	}
	return out
}

func (a *sourceAdmin) link(ctx context.Context, externalID string, company ids.CompanyID) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"company_id":%q}`, company.String())
	return call(ctx, http.MethodPut, body, func(w http.ResponseWriter, r *http.Request) {
		a.handlers.LinkFinanceSourceCustomer(w, r, externalID)
	})
}

func (a *sourceAdmin) unlink(ctx context.Context, externalID string) *httptest.ResponseRecorder {
	return call(ctx, http.MethodDelete, "", func(w http.ResponseWriter, r *http.Request) {
		a.handlers.UnlinkFinanceSourceCustomer(w, r, externalID)
	})
}

func linkedName(entry crmcontracts.FinanceSourceCustomer) string {
	if entry.Company == nil {
		return ""
	}
	return entry.Company.DisplayName
}

func TestTheDirectoryPagesInNameOrderAndNamesTheLinkedCompany(t *testing.T) {
	a := setupSourceAdmin(t)
	a.syncDirectory(t)
	ctx := a.admin()

	rec := a.list(ctx, nil, 2)
	expect(t, rec, http.StatusOK, "")
	first := decode[crmcontracts.FinanceSourceCustomerListResponse](t, rec)
	if len(first.Data) != 2 || first.Data[0].ExternalId != "ACME-01" || first.Data[1].ExternalId != "BETA-02" {
		t.Fatalf("first page = %+v, want ACME-01 then BETA-02", first.Data)
	}
	if got := linkedName(first.Data[0]); got != "Ledger GmbH" {
		t.Errorf("ACME-01 is linked to %q, want the fixture's company", got)
	}
	if !first.Page.HasMore || first.Page.NextCursor == nil {
		t.Fatalf("page = %+v, want a cursor to the rest", first.Page)
	}

	second := decode[crmcontracts.FinanceSourceCustomerListResponse](t, a.list(ctx, first.Page.NextCursor, 2))
	if len(second.Data) != 1 || second.Data[0].ExternalId != "CETO-03" || second.Page.HasMore {
		t.Errorf("second page = %+v, want CETO-03 and the end", second)
	}

	forged := "not-a-cursor"
	expect(t, a.list(ctx, &forged, 2), http.StatusBadRequest, "invalid_argument")

	// Without the company grant the directory still lists, but names no company.
	blind := a.as(principal.RowScopeAll, map[string]principal.ObjectGrant{objectIntegrations: {Read: true}})
	if got := linkedName(a.entries(blind, t)["ACME-01"]); got != "" {
		t.Errorf("a caller without the company grant was told ACME-01 is linked to %q", got)
	}
	expect(t, a.list(a.as(principal.RowScopeAll, nil), nil, 2), http.StatusForbidden, "permission_denied")

	expect(t, call(ctx, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusNoContent, "")
	if got := a.entries(ctx, t); len(got) != 0 {
		t.Errorf("a disconnected source still lists %d customers", len(got))
	}
}

// Relinking moves the customer, linking it where it already is writes nothing,
// and unlinking takes it off. Each change is one audit row.
func TestLinkingMovesACustomerAndUnlinkingTakesItOff(t *testing.T) {
	a := setupSourceAdmin(t)
	a.syncDirectory(t)
	ctx := a.admin()
	beta := a.seedCompany(t, "Beta GmbH", nil)
	mark := auditWatermark(a.ctx, t, a.financeEnv)
	linkAudits := func() int { return auditRowsByEntity(a.ctx, t, a.financeEnv, mark)[entityCustomerLink] }

	rec := a.link(ctx, "BETA-02", beta)
	expect(t, rec, http.StatusOK, "")
	if got := decode[crmcontracts.FinanceSourceCustomer](t, rec); got.DisplayName != "Beta Corp" || linkedName(got) != "Beta GmbH" {
		t.Fatalf("link answered %+v, want Beta Corp on Beta GmbH", got)
	}
	expect(t, a.link(ctx, "BETA-02", beta), http.StatusOK, "")
	if got := linkAudits(); got != 1 {
		t.Errorf("%d link audit rows after linking twice to one company, want 1", got)
	}

	expect(t, a.link(ctx, "BETA-02", a.company), http.StatusOK, "")
	if got := linkedName(a.entries(ctx, t)["BETA-02"]); got != "Ledger GmbH" {
		t.Errorf("BETA-02 is on %q after the relink, want Ledger GmbH", got)
	}

	expect(t, a.unlink(ctx, "BETA-02"), http.StatusNoContent, "")
	if got := linkedName(a.entries(ctx, t)["BETA-02"]); got != "" {
		t.Errorf("BETA-02 is still on %q after the unlink", got)
	}
	expect(t, a.unlink(ctx, "BETA-02"), http.StatusNoContent, "")
	if got := linkAudits(); got != 3 {
		t.Errorf("%d link audit rows, want create, update and archive", got)
	}
}

func TestALinkRequestNamesAKnownCustomerAndCompany(t *testing.T) {
	a := setupSourceAdmin(t)
	a.syncDirectory(t)
	ctx := a.admin()

	missing := call(ctx, http.MethodPut, `{}`, func(w http.ResponseWriter, r *http.Request) {
		a.handlers.LinkFinanceSourceCustomer(w, r, "BETA-02")
	})
	expect(t, missing, http.StatusUnprocessableEntity, "validation_error")
	expect(t, a.link(ctx, "NOPE-99", a.company), http.StatusNotFound, "not_found")
	expect(t, a.unlink(ctx, "NOPE-99"), http.StatusNotFound, "not_found")
	expect(t, a.link(ctx, "BETA-02", ids.New[ids.CompanyKind]()), http.StatusNotFound, "not_found")

	noCompany := a.as(principal.RowScopeAll, map[string]principal.ObjectGrant{objectIntegrations: everyAction})
	expect(t, a.link(noCompany, "BETA-02", a.company), http.StatusForbidden, "permission_denied")

	expect(t, call(ctx, http.MethodDelete, "", a.handlers.DisconnectFinanceSource), http.StatusNoContent, "")
	expect(t, a.link(ctx, "BETA-02", a.company), http.StatusNotFound, "not_found")
	expect(t, a.unlink(ctx, "ACME-01"), http.StatusNotFound, "not_found")
}

// A link puts money on a company's account, so it needs write authority on
// that company, not only sight of it. A member sees another seat's company in
// the directory, and still may not link onto it or take a customer off it.
func TestALinkNeedsWriteAuthorityOnTheCompany(t *testing.T) {
	a := setupSourceAdmin(t)
	a.syncDirectory(t)
	stranger := a.seedUser(t)
	theirs := a.seedCompany(t, "Stranger GmbH", &stranger)
	expect(t, a.link(a.admin(), "CETO-03", theirs), http.StatusOK, "")

	member := a.as(principal.RowScopeOwn, map[string]principal.ObjectGrant{
		objectIntegrations: everyAction, "company": everyAction,
	})
	if got := linkedName(a.entries(member, t)["CETO-03"]); got != "Stranger GmbH" {
		t.Errorf("CETO-03 reads %q to the member, want the company it may see", got)
	}
	expect(t, a.link(member, "BETA-02", theirs), http.StatusForbidden, "permission_denied")
	expect(t, a.unlink(member, "CETO-03"), http.StatusForbidden, "permission_denied")
	expect(t, a.link(member, "CETO-03", a.company), http.StatusForbidden, "permission_denied")

	owner := a.asUser(stranger, principal.RowScopeOwn, map[string]principal.ObjectGrant{
		objectIntegrations: everyAction, "company": everyAction,
	})
	expect(t, a.unlink(owner, "CETO-03"), http.StatusNoContent, "")
}

// The ledgers are read with no transaction open, so the database can change
// under a pass. A link removed meanwhile mirrors nothing.
func TestALinkRemovedWhileTheLedgersAreReadMirrorsNothing(t *testing.T) {
	a := setupSourceAdmin(t)
	ctx := a.admin()
	source := directorySource{customers: directory, onLedger: func() {
		if rec := a.unlink(ctx, "ACME-01"); rec.Code != http.StatusNoContent {
			t.Errorf("unlinking mid-pass answered %d: %s", rec.Code, rec.Body.String())
		}
	}}

	result, err := a.store.SyncConnection(a.ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if result.InvoicesInsert != 0 {
		t.Errorf("%d invoices mirrored for a customer unlinked during the pass, want 0", result.InvoicesInsert)
	}
	if status := a.connectionStatus(t, a.connection); status != statusConnectionActive {
		t.Errorf("connection status = %q, want the pass marked %q", status, statusConnectionActive)
	}
}

// Marking a pass synced would reconnect a source the administrator disconnected
// while its ledgers were being read.
func TestASourceDisconnectedDuringAPassStaysDisconnected(t *testing.T) {
	a := setupSourceAdmin(t)
	ctx := a.admin()
	source := directorySource{customers: directory, onLedger: func() {
		if rec := call(ctx, http.MethodDelete, "", a.handlers.DisconnectFinanceSource); rec.Code != http.StatusNoContent {
			t.Errorf("disconnecting mid-pass answered %d: %s", rec.Code, rec.Body.String())
		}
	}}

	result, err := a.store.SyncConnection(a.ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if result.InvoicesInsert != 0 {
		t.Errorf("%d invoices mirrored from a source disconnected during the pass, want 0", result.InvoicesInsert)
	}
	if status := a.connectionStatus(t, a.connection); status != statusDisconnected {
		t.Errorf("connection status = %q after the pass, want it left %q", status, statusDisconnected)
	}

	// With nothing connected, the next pass writes nothing and answers no error.
	if result, err := a.store.SyncConnection(a.ctx, source); err != nil || result != (SyncResult{}) {
		t.Errorf("a pass with no connection answered %+v, %v; want nothing done and no error", result, err)
	}
}

type unreadableLedger struct{ directorySource }

var errLedgerRefused = errors.New("the ledger read timed out")

func (unreadableLedger) InvoicesFor(context.Context, string) (SourceLedger, error) {
	return SourceLedger{}, errLedgerRefused
}

func TestALedgerTheSourceCannotReadFailsThePass(t *testing.T) {
	a := setupSourceAdmin(t)
	_, err := a.store.SyncConnection(a.ctx, unreadableLedger{directorySource{customers: directory}})
	if !errors.Is(err, errLedgerRefused) {
		t.Fatalf("the pass answered %v, want the ledger's own failure", err)
	}
}
