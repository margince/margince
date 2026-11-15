// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"math"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCustomerPaymentTotalsKeepEachCurrencyAndCountACompanyOnce(t *testing.T) {
	fold := newPaymentFold()
	company := ids.NewV7()
	for _, invoice := range []crmcontracts.CustomerPaymentInvoice{
		{CompanyId: openapi_types.UUID(company), CompanyName: "Acme", Currency: "EUR", OpenMinor: 1200, DaysOverdue: 4},
		{CompanyId: openapi_types.UUID(company), CompanyName: "Acme", Currency: "USD", OpenMinor: 1700, DaysOverdue: 8},
		{CompanyId: openapi_types.UUID(company), CompanyName: "Acme", Currency: "EUR", OpenMinor: 300, DaysOverdue: 2},
	} {
		if err := fold.add(invoice); err != nil {
			t.Fatal(err)
		}
	}
	out := crmcontracts.CustomerPayments{}
	fold.fill(&out, ids.NewV7())
	if out.InvoiceCount != 3 || out.CompanyCount != 1 || len(out.Totals) != 2 ||
		out.Totals[0].Currency != "EUR" || out.Totals[0].AmountMinor != 1500 ||
		out.Totals[1].Currency != "USD" || out.Totals[1].AmountMinor != 1700 {
		t.Fatalf("currencies or distinct customer count merged: %+v", out)
	}
	if len(out.Customers) != 2 || out.Customers[0].OldestDays != 8 {
		t.Fatalf("oldest customer group is not first: %+v", out.Customers)
	}
}

func TestCustomerPaymentPreviewsStopAtThreeButTotalsCountEveryInvoice(t *testing.T) {
	fold := newPaymentFold()
	for n := 1; n <= 5; n++ {
		if err := fold.add(crmcontracts.CustomerPaymentInvoice{CompanyId: openapi_types.UUID(ids.NewV7()), Currency: "EUR", OpenMinor: 100, DaysOverdue: n}); err != nil {
			t.Fatal(err)
		}
	}
	out := crmcontracts.CustomerPayments{}
	fold.fill(&out, ids.NewV7())
	if out.InvoiceCount != 5 || out.CompanyCount != 5 || out.Totals[0].AmountMinor != 500 || len(out.Customers) != 3 || out.Customers[0].OldestDays != 5 || out.Customers[2].OldestDays != 3 {
		t.Fatalf("preview cap changed the totals or order: %+v", out)
	}
}

func TestCustomerPaymentTotalsRefuseOverflow(t *testing.T) {
	fold := newPaymentFold()
	invoice := crmcontracts.CustomerPaymentInvoice{CompanyId: openapi_types.UUID(ids.NewV7()), Currency: "EUR", OpenMinor: math.MaxInt64}
	if err := fold.add(invoice); err != nil {
		t.Fatal(err)
	}
	invoice.OpenMinor = 1
	if err := fold.add(invoice); err == nil {
		t.Fatal("overflow wrapped a complete financial total")
	}
}
