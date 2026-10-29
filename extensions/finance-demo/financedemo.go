// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package financedemo is the demonstration accounting source. Its ledgers are
// generated rather than fetched, so the finance surfaces can be shown before a
// real accounting system is connected.
//
// It is also the reference for a FinanceSource. A real source calls a vendor
// where this one generates. The declaration and the records it answers are the
// same. Its provider is `offline_demo` on every row and on the
// card, because a figure a reader cannot tell from a real one is worse than
// none.
//
// Its OAuth app is the reference for an OAuthApp. Its vendor is a reserved
// example host, so an admin who connects it lands on a failure, not a vendor.
package financedemo

import (
	"context"
	"embed"
	"fmt"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/accounting"
)

//go:embed migrations
var migrations embed.FS

// directorySize is how many demo customers the source lists, enough to link
// every customer company the demo seed creates.
const directorySize = 50

// New returns the unit's declaration.
func New() extension.Extension {
	return extension.Extension{
		Name:        "finance-demo",
		Version:     "1.0.0",
		Description: "Demonstration accounting data for the finance card, generated rather than read from a real system.",
		// The reference for a company type: an account whose invoices this
		// source's ledger holds.
		CompanyTypes: []extension.CompanyType{{Type: "ledger_customer", Label: "Ledger customer"}},
		// The reference for a record kind: a ledger account, kept in the unit's
		// own table. No role holds a right on it until an admin grants one,
		// except the starting grants named here.
		RecordKinds: []extension.RecordKindDeclaration{{
			Kind:        "ledger_account",
			DisplayName: "Ledger account",
			Table:       "ext_finance_demo_ledger_account",
			Fields: []extension.RecordKindField{
				{Name: "account_code", Label: "Account code"},
				{Name: "name", Label: "Name"},
			},
			NaturalKey: []string{"account_code"},
			Defaults: []extension.RbacDefault{
				{Role: "admin", Actions: []extension.RbacAction{extension.RbacRead, extension.RbacCreate, extension.RbacUpdate, extension.RbacDelete}},
				{Role: "manager", Actions: []extension.RbacAction{extension.RbacRead}},
			},
		}},
		// The reference for a report: how many ledger accounts exist, drawn on
		// Reports and Home for a seat that may read the kind.
		ReportCounts:     []extension.ReportCount{{Name: "ledger_accounts", Label: "Ledger accounts", Population: "ledger_account"}},
		ReportBreakdowns: []extension.ReportBreakdown{{Name: "accounts_by_name", Label: "Ledger accounts by name", Population: "ledger_account", By: "name"}},
		Migrations:       migrations,
		FinanceSources: []extension.FinanceSource{{
			Provider:    "offline_demo",
			Check:       check,
			Customers:   customers,
			InvoicesFor: invoicesFor,
		}},
		OAuthApps: []extension.OAuthApp{{ //nolint:gosec // G101: TokenURL is an endpoint address, not a credential
			Provider: "offline_demo",
			Title:    "Demo accounting sign-in (example only, cannot connect)",
			AuthURL:  "https://auth.ledger.example/authorize",
			TokenURL: "https://auth.ledger.example/token",
			Scopes:   []string{"ledger.read"},
		}},
	}
}

// check always answers yes: the generator needs no credential and no network.
func check(context.Context, extension.Runtime, accounting.Connection) error { return nil }

// customers lists a fixed directory, DEMO-0001 onwards, which an administrator
// links onto companies like any source's customers.
func customers(context.Context, extension.Runtime, accounting.Connection) ([]accounting.Customer, error) {
	out := make([]accounting.Customer, 0, directorySize)
	for n := 1; n <= directorySize; n++ {
		out = append(out, accounting.Customer{
			ExternalID:  fmt.Sprintf("DEMO-%04d", n),
			DisplayName: fmt.Sprintf("Demo customer %02d", n),
		})
	}
	return out, nil
}

// invoicesFor generates the ledger of any customer id, listed or not.
func invoicesFor(_ context.Context, _ extension.Runtime, conn accounting.Connection, externalCustomerID string) (accounting.Ledger, error) {
	return generate(conn, externalCustomerID)
}
