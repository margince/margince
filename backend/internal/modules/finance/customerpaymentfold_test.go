// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"math"
	"testing"
	"time"

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
	if len(out.Customers) != 2 || out.Customers[0].OldestDays != 4 {
		t.Fatalf("recent customer group is not first: %+v", out.Customers)
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
	if out.InvoiceCount != 5 || out.CompanyCount != 5 || out.Totals[0].AmountMinor != 500 || len(out.Customers) != 3 || out.Customers[0].OldestDays != 1 || out.Customers[2].OldestDays != 3 {
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

func TestPaymentAgeGroupsOnlyOlderInvoicesAndKeepsCompleteTotals(t *testing.T) {
	for _, age := range []int{30, 365, 1000} {
		fold := newPaymentFold()
		fold.age = age
		company := openapi_types.UUID(ids.NewV7())
		for _, days := range []int{30, 365, 366, 1000} {
			if err := fold.add(crmcontracts.CustomerPaymentInvoice{CompanyId: company, Currency: "EUR", OpenMinor: 100, DaysOverdue: days}); err != nil {
				t.Fatal(err)
			}
		}
		out := crmcontracts.CustomerPayments{}
		fold.fill(&out, ids.NewV7())
		expected := 0
		for _, days := range []int{30, 365, 366, 1000} {
			if days > age {
				expected++
			}
		}
		if out.InvoiceCount != 4 || out.CompanyCount != 1 || out.Totals[0].AmountMinor != 400 || out.LongOverdue == nil || out.LongOverdue.InvoiceCount != expected {
			t.Fatalf("age%d changed totals or group: %+v", age, out)
		}
		for _, customer := range out.Customers {
			if customer.OldestDays > age {
				t.Fatalf("age%d leaked old invoice into active group: %+v", age, customer)
			}
		}
	}
}

func TestPaymentCursorFollowsRecentInvoicesAndLargeSameDayAmounts(t *testing.T) {
	due := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	cursor := paymentPosition{Connection: ids.NewV7(), Due: due, Invoice: ids.NewV7(), Currency: "EUR", Amount: 200}
	for _, tc := range []struct {
		name    string
		invoice crmcontracts.CustomerPaymentInvoice
		want    bool
	}{
		{"older", crmcontracts.CustomerPaymentInvoice{DueAt: openapi_types.Date{Time: due.AddDate(0, 0, -1)}}, true},
		{"newer", crmcontracts.CustomerPaymentInvoice{DueAt: openapi_types.Date{Time: due.AddDate(0, 0, 1)}}, false},
		{"smaller", crmcontracts.CustomerPaymentInvoice{DueAt: openapi_types.Date{Time: due}, Currency: "EUR", OpenMinor: 100}, true},
		{"larger", crmcontracts.CustomerPaymentInvoice{DueAt: openapi_types.Date{Time: due}, Currency: "EUR", OpenMinor: 300}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := followsPayment(tc.invoice, cursor); got != tc.want {
				t.Fatalf("follows=%v want%v", got, tc.want)
			}
		})
	}
}
