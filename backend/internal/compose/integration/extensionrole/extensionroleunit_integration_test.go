// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package extensionrole

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/accounting"
	"github.com/margince/margince/backend/pkg/extension/compliancedocs"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
)

const (
	// unitLedger is the accounting source the synthetic unit offers.
	unitLedger = "roleext_ledger"
	// unitCustomer is the customer the link verbs act on.
	unitCustomer = "RX-1"
	// billedCustomer is the customer whose one invoice the document verb opens.
	billedCustomer = "RX-2"
	// billedInvoice is the billed customer's one invoice.
	billedInvoice = billedCustomer + "-INV-1"
	// unitDocument is the compliance document it ships.
	unitDocument = "roleext-note"
)

// unitJurisdiction is the jurisdiction the unit's document is for: the test
// pack the unit carries, which a country can then be set to.
var unitJurisdiction = integration.GoBDFloorPack{}.Code()

// syntheticUnit is a unit with an accounting source of two customers, one of
// them billed, and one document. The verbs that read either reach their gates.
func syntheticUnit() extension.Extension {
	return extension.Extension{
		Name: unitName, Version: "1.0.0", Description: "A synthetic unit, composed only for this suite's own test binary.",
		FinanceSources: []extension.FinanceSource{{
			Provider: unitLedger,
			Check:    func(context.Context, extension.Runtime, accounting.Connection) error { return nil },
			Customers: func(context.Context, extension.Runtime, accounting.Connection) ([]accounting.Customer, error) {
				return []accounting.Customer{
					{ExternalID: unitCustomer, DisplayName: "Role Sweep Ledger"},
					{ExternalID: billedCustomer, DisplayName: "Role Sweep Billed"},
				}, nil
			},
			InvoicesFor: billedLedger,
			InvoiceDocument: func(context.Context, extension.Runtime, accounting.Connection, string) (accounting.Document, error) {
				return accounting.Document{ContentType: "application/pdf", Body: []byte("%PDF-1.4 role sweep\n")}, nil
			},
		}},
		Jurisdictions: []jurisdiction.Pack{integration.GoBDFloorPack{}},
		Documents: []compliancedocs.Document{{
			Jurisdiction: jurisdiction.Code(unitJurisdiction), ID: unitDocument, Title: "Role sweep note", Language: "en",
			Audience: compliancedocs.Operator, Kind: compliancedocs.ReadingCopy, Body: []byte("# Role sweep\n"), MediaType: "text/markdown",
		}},
	}
}

// billedLedger answers one invoice for the billed customer and none for another.
func billedLedger(_ context.Context, _ extension.Runtime, _ accounting.Connection, customer string) (accounting.Ledger, error) {
	if customer != billedCustomer {
		return accounting.Ledger{}, nil
	}
	issued := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return accounting.Ledger{Invoices: []accounting.Invoice{{
		ExternalID: billedInvoice, Number: "RX-INV-1", IssuedOn: issued, Currency: "EUR", NetMinor: 10000, GrossMinor: 10000,
	}}}, nil
}

// seedUnitState sets the installation's country to the unit's jurisdiction and
// connects its accounting source, as an administrator does. It makes a company
// to link a customer to, bills another company through a linked customer, and
// answers the arguments of each verb it serves.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func seedUnitState(t *testing.T, e *apptest.AppEnv) map[string]map[string]any {
	t.Helper()
	for _, step := range []struct {
		method, path string
		body         integration.AnyMap
	}{
		{"PATCH", "/v1/installation/settings", integration.AnyMap{"country": string(unitJurisdiction)}},
		{"PUT", "/v1/finance/source", integration.AnyMap{"provider": unitLedger}},
	} {
		if status := e.Call(t, step.method, step.path, step.body, nil, nil); status >= 300 {
			t.Fatalf("%s %s → %d", step.method, step.path, status)
		}
	}
	workFinanceSyncs(t, e)
	company := createCompany(t, e, "Ledger Co")
	billed := createCompany(t, e, "Billed Co")
	if status := e.Call(t, "PUT", "/v1/finance/source/customers/"+billedCustomer+"/link", integration.AnyMap{"company_id": billed}, nil, nil); status >= 300 {
		t.Fatalf("linking the billed customer → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/finance/source/sync", nil, nil, nil); status >= 300 {
		t.Fatalf("queueing the sync that mirrors the invoice → %d", status)
	}
	workFinanceSyncs(t, e)
	var invoice string
	if err := e.Owner.QueryRow(t.Context(), `SELECT i.id::text FROM finance_invoice i
		   JOIN finance_connection c ON c.id = i.connection_id
		  WHERE i.external_id = $1 AND c.archived_at IS NULL AND c.status <> 'disconnected'`, billedInvoice).Scan(&invoice); err != nil {
		t.Fatalf("the sync mirrored no invoice for the billed customer: %v", err)
	}
	return map[string]map[string]any{
		"download_finance_invoice_pdf":   {"id": invoice},
		"download_jurisdiction_document": {"documentId": unitDocument},
		"link_finance_customer":          {"external_id": unitCustomer, "request_body": map[string]any{"company_id": company}},
		"unlink_finance_customer":        {"external_id": unitCustomer},
		"sync_finance_source":            {},
	}
}

// workFinanceSyncs works the queued finance syncs as the worker role does,
// which reads the source's customers and linked ledgers into the mirror.
func workFinanceSyncs(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	worker, err := compose.FinanceSyncWorkerForTest(e.Pool)
	if err != nil {
		t.Fatalf("building the finance sync worker: %v", err)
	}
	for _, job := range importjobtest.Queued(t, e, compose.FinanceSyncArgs{}.Kind()) {
		importjobtest.WorkJob(t, e, worker, job, 0)
	}
}

func createCompany(t *testing.T, e *apptest.AppEnv, name string) string {
	t.Helper()
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", integration.AnyMap{"display_name": name, "source": "manual"}, nil, &company); status != 201 {
		t.Fatalf("creating %s → %d", name, status)
	}
	return company.ID
}
