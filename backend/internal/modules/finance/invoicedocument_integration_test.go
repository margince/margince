// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// documentReader may read finance and see every company.
func (a *sourceAdmin) documentReader(scope principal.RowScope) context.Context {
	return a.as(scope, map[string]principal.ObjectGrant{
		"finance": {Read: true}, "company": {Read: true},
	})
}

// mirroredInvoice syncs the fixture ledger and answers the newest invoice the
// card lists.
func (a *sourceAdmin) mirroredInvoice(t *testing.T) crmcontracts.FinanceInvoice {
	t.Helper()
	if _, err := a.store.SyncConnection(a.ctx, a.provider()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	summary := a.summaryAtEpoch(t, a.company)
	if summary.InvoiceDocuments == nil || !*summary.InvoiceDocuments {
		t.Fatalf("the card does not offer documents from a source that serves them: %+v", summary.InvoiceDocuments)
	}
	if summary.RecentInvoices == nil || len(*summary.RecentInvoices) == 0 {
		t.Fatal("the sync mirrored no invoice; the rest of this test proves nothing")
	}
	return (*summary.RecentInvoices)[0]
}

func download(ctx context.Context, h Handlers, id ids.UUID) *httptest.ResponseRecorder {
	return call(ctx, http.MethodGet, "", func(w http.ResponseWriter, r *http.Request) {
		h.DownloadFinanceInvoicePdf(w, r, crmcontracts.Id(id))
	})
}

func TestAnInvoiceOpensAsTheSourcesOwnPDF(t *testing.T) {
	a := setupSourceAdmin(t)
	invoice := a.mirroredInvoice(t)

	rec := download(a.documentReader(principal.RowScopeAll), a.handlers, ids.UUID(invoice.Id))
	expect(t, rec, http.StatusOK, "")
	if got := rec.Header().Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", got)
	}
	if got, want := rec.Header().Get("Content-Disposition"), `inline; filename="invoice-`+*invoice.Number+`.pdf"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store: the document is read under one caller's grant", got)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF-1.4 ") {
		t.Errorf("body = %q, want the fixture source's PDF", rec.Body.String())
	}
}

// A caller who cannot see the company learns nothing about its invoices, not
// even that one exists.
func TestAnInvoiceOfAHiddenCompanyAnswersAsIfItDidNotExist(t *testing.T) {
	a := setupSourceAdmin(t)
	invoice := a.mirroredInvoice(t)

	a.makeOwnerPrivate(t)
	hidden := download(a.documentReader(principal.RowScopeOwn), a.handlers, ids.UUID(invoice.Id))
	missing := download(a.documentReader(principal.RowScopeAll), a.handlers, ids.NewV7())
	expect(t, hidden, http.StatusNotFound, "not_found")
	expect(t, missing, http.StatusNotFound, "not_found")
	if hidden.Body.String() != missing.Body.String() {
		t.Errorf("a hidden invoice answers %s, a missing one %s; the two must not differ", hidden.Body, missing.Body)
	}

	noGrant := a.as(principal.RowScopeAll, map[string]principal.ObjectGrant{"company": {Read: true}})
	expect(t, download(noGrant, a.handlers, ids.UUID(invoice.Id)), http.StatusForbidden, "permission_denied")
}

// An archived company's card is not found, and neither are the documents it
// listed, though archiving leaves its invoices in place.
func TestAnInvoiceOfAnArchivedCompanyIsNotFound(t *testing.T) {
	a := setupSourceAdmin(t)
	invoice := a.mirroredInvoice(t)

	a.ownerExec(t, `UPDATE company SET archived_at = now() WHERE id = $1`, a.company)
	expect(t, download(a.documentReader(principal.RowScopeAll), a.handlers, ids.UUID(invoice.Id)),
		http.StatusNotFound, "not_found")
}

// makeOwnerPrivate gives the fixture company an owner and restricts it to them,
// so a reader with only their own rows cannot see it.
func (a *sourceAdmin) makeOwnerPrivate(t *testing.T) {
	t.Helper()
	owner := ids.NewV7()
	a.ownerExec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Invoice owner')`,
		owner, owner.String()+"@example.test")
	a.ownerExec(t, `UPDATE company SET owner_id = $1, captured_by = $2, visibility = 'owner' WHERE id = $3`,
		owner, "human:"+owner.String(), a.company)
}

// Once the installation reads a newer source, an invoice the earlier one issued
// is no longer that source's to open.
func TestAnInvoiceFromAReplacedConnectionIsNotFound(t *testing.T) {
	a := setupSourceAdmin(t)
	invoice := a.mirroredInvoice(t)
	successor := ids.NewV7()
	a.ownerExec(t, `
		INSERT INTO finance_connection (id, provider, status, credential_ref, source, captured_by, created_at)
		VALUES ($1, $2, 'active', 'offline://successor', 'system', 'system:test', now() + interval '1 second')`,
		successor, fixtureProviderName)
	// The package shares one database, and every later test reads the newest connection.
	t.Cleanup(func() {
		a.ownerExec(t, `UPDATE finance_connection SET archived_at = now() WHERE id = $1`, successor)
	})

	expect(t, download(a.documentReader(principal.RowScopeAll), a.handlers, ids.UUID(invoice.Id)),
		http.StatusNotFound, "not_found")
}

// documentlessSources offers the suite's ledger from a unit that serves no
// documents, or one that answers with something other than a PDF.
type documentlessSources struct{ document Provider }

func (documentlessSources) Providers() []SourceOption {
	return []SourceOption{{Provider: fixtureProviderName}}
}

func (s documentlessSources) Open(string, SourceConnection) (Provider, bool) { return s.document, true }

type withoutDocuments struct{ fixtureSource }

func (withoutDocuments) InvoiceDocument(context.Context, string) (SourceDocument, error) {
	return SourceDocument{}, apperrors.ErrUnsupportedBySoR
}

type pageInsteadOfPDF struct{ fixtureSource }

func (pageInsteadOfPDF) InvoiceDocument(context.Context, string) (SourceDocument, error) {
	return SourceDocument{ContentType: "text/html", Body: []byte("<script>alert(1)</script>")}, nil
}

func TestASourceWithoutDocumentsIsNeitherOfferedNorOpened(t *testing.T) {
	a := setupSourceAdmin(t)
	invoice := a.mirroredInvoice(t)
	a.store.WithSources(documentlessSources{document: withoutDocuments{}})

	if summary := a.summaryAtEpoch(t, a.company); summary.InvoiceDocuments == nil || *summary.InvoiceDocuments {
		t.Errorf("invoice_documents = %v, want false for a source that serves none", summary.InvoiceDocuments)
	}
	expect(t, download(a.documentReader(principal.RowScopeAll), a.handlers, ids.UUID(invoice.Id)),
		http.StatusUnprocessableEntity, "unsupported_by_sor")
}

// Only a PDF leaves the core. Anything else from a source could render as a
// page of this origin.
func TestADocumentThatIsNotAPDFIsRefused(t *testing.T) {
	a := setupSourceAdmin(t)
	invoice := a.mirroredInvoice(t)
	a.store.WithSources(documentlessSources{document: pageInsteadOfPDF{}})

	rec := download(a.documentReader(principal.RowScopeAll), a.handlers, ids.UUID(invoice.Id))
	expect(t, rec, http.StatusBadGateway, "provider_unusable")
	if strings.Contains(rec.Body.String(), "script") {
		t.Errorf("the refusal echoes the source's document: %s", rec.Body)
	}
}
