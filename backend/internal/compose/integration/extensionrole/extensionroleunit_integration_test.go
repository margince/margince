// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package extensionrole

import (
	"context"
	"testing"

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
	// unitCustomer is the one customer its directory holds.
	unitCustomer = "RX-1"
	// unitDocument is the compliance document it ships.
	unitDocument = "roleext-note"
)

// unitJurisdiction is the jurisdiction the unit's document is for: the test
// pack the unit carries, which a country can then be set to.
var unitJurisdiction = integration.GoBDFloorPack{}.Code()

// syntheticUnit is a unit with an accounting source of one customer and one
// document, so the verbs that read either reach their gates.
func syntheticUnit() extension.Extension {
	return extension.Extension{
		Name: unitName, Version: "1.0.0", Description: "A synthetic unit, composed only for this suite's own test binary.",
		FinanceSources: []extension.FinanceSource{{
			Provider: unitLedger,
			Check:    func(context.Context, extension.Runtime, accounting.Connection) error { return nil },
			Customers: func(context.Context, extension.Runtime, accounting.Connection) ([]accounting.Customer, error) {
				return []accounting.Customer{{ExternalID: unitCustomer, DisplayName: "Role Sweep Ledger"}}, nil
			},
			InvoicesFor: func(context.Context, extension.Runtime, accounting.Connection, string) (accounting.Ledger, error) {
				return accounting.Ledger{}, nil
			},
		}},
		Jurisdictions: []jurisdiction.Pack{integration.GoBDFloorPack{}},
		Documents: []compliancedocs.Document{{
			Jurisdiction: jurisdiction.Code(unitJurisdiction), ID: unitDocument, Title: "Role sweep note", Language: "en",
			Audience: compliancedocs.Operator, Kind: compliancedocs.ReadingCopy, Body: []byte("# Role sweep\n"), MediaType: "text/markdown",
		}},
	}
}

// seedUnitState sets the installation's country to the unit's jurisdiction and
// connects its accounting source, as an administrator does. It makes a company
// to link a customer to, and answers the arguments of each verb it serves.
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
	// The source's customers are read into the mirror by the sync the
	// connection queued, worked here as the worker role works it.
	worker, err := compose.FinanceSyncWorkerForTest(e.Pool)
	if err != nil {
		t.Fatalf("building the finance sync worker: %v", err)
	}
	for _, job := range importjobtest.Queued(t, e, compose.FinanceSyncArgs{}.Kind()) {
		importjobtest.WorkJob(t, e, worker, job, 0)
	}
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", integration.AnyMap{"display_name": "Ledger Co", "source": "manual"}, nil, &company); status != 201 {
		t.Fatalf("creating the company to link → %d", status)
	}
	return map[string]map[string]any{
		"download_jurisdiction_document": {"documentId": unitDocument},
		"link_finance_customer":          {"external_id": unitCustomer, "request_body": map[string]any{"company_id": company.ID}},
		"unlink_finance_customer":        {"external_id": unitCustomer},
		"sync_finance_source":            {},
	}
}
