// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type paymentGroup struct {
	company  ids.UUID
	currency string
}
type paymentFold struct {
	linkedAt  time.Time
	linked    map[ids.UUID]bool
	companies map[ids.UUID]bool
	totals    map[string]int64
	customers map[paymentGroup]crmcontracts.CustomerPaymentCustomer
	invoices  int
 age int
 aged *paymentFold
}

func newPaymentFold() *paymentFold {
	return &paymentFold{
		age: 365, linked: map[ids.UUID]bool{}, companies: map[ids.UUID]bool{},
		totals: map[string]int64{}, customers: map[paymentGroup]crmcontracts.CustomerPaymentCustomer{},
	}
}

func (fold *paymentFold) admit(row paymentRow, out *crmcontracts.CustomerPayments) bool {
	if !collectsFrom(row.lifecycle) {
		return false
	}
	fold.linked[row.companyID] = true
	if row.linkedAt.After(fold.linkedAt) {
		fold.linkedAt = row.linkedAt
	}
	if out.LastSyncedAt == nil {
		return false
	}
	if row.linkedAt.After(*out.LastSyncedAt) {
		if out.State != crmcontracts.FinanceSummaryStateError {
			out.State = crmcontracts.FinanceSummaryStateSyncing
		}
		return false
	}
	return out.State != crmcontracts.FinanceSummaryStateSyncing && row.invoiceID != nil
}

func (fold *paymentFold) add(invoice crmcontracts.CustomerPaymentInvoice) error {
	total := fold.totals[invoice.Currency]
	if invoice.OpenMinor > math.MaxInt64-total {
		return errors.New("customer payment total exceeds the supported amount")
	}
	fold.totals[invoice.Currency] = total + invoice.OpenMinor
	fold.invoices++
	company := ids.UUID(invoice.CompanyId)
	fold.companies[company] = true
 if fold.age > 0 && invoice.DaysOverdue > fold.age {
  if fold.aged == nil { fold.aged = newPaymentFold(); fold.aged.age = 0 }
  return fold.aged.add(invoice)
 }
 key := paymentGroup{company, invoice.Currency}
	group := fold.customers[key]
	group.CompanyId, group.CompanyName, group.Currency = invoice.CompanyId, invoice.CompanyName, invoice.Currency
	group.AmountMinor += invoice.OpenMinor
	group.InvoiceCount++
	if invoice.DaysOverdue > group.OldestDays {
		group.OldestDays = invoice.DaysOverdue
	}
	fold.customers[key] = group
	return nil
}

func (fold *paymentFold) fill(out *crmcontracts.CustomerPayments, connectionID ids.UUID) {
	visible := make([]string, 0, len(fold.linked))
	for id := range fold.linked {
		visible = append(visible, id.String())
	}
	sort.Strings(visible)
	snapshot := connectionID.String() + string(out.State) + fold.linkedAt.UTC().Format(time.RFC3339Nano) + strings.Join(visible, "|")
	if out.LastSyncedAt != nil {
		snapshot += out.LastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	digest := sha256.Sum256([]byte(snapshot))
	out.AccessScope = hex.EncodeToString(digest[:])
	out.LinkedCompanies, out.InvoiceCount, out.CompanyCount = len(fold.linked), fold.invoices, len(fold.companies)
	for currency, amount := range fold.totals {
		out.Totals = append(out.Totals, crmcontracts.CustomerPaymentTotal{Currency: currency, AmountMinor: amount})
	}
	sort.Slice(out.Totals, func(i, j int) bool { return out.Totals[i].Currency < out.Totals[j].Currency })
	for _, group := range fold.customers {
		out.Customers = append(out.Customers, group)
	}
	sort.Slice(out.Customers, func(i, j int) bool {
		a, b := out.Customers[i], out.Customers[j]
		if a.OldestDays != b.OldestDays {
			return a.OldestDays < b.OldestDays
		}
  if a.Currency == b.Currency && a.AmountMinor != b.AmountMinor { return a.AmountMinor > b.AmountMinor }
  if a.CompanyId != b.CompanyId {
			return a.CompanyId.String() < b.CompanyId.String()
		}
		return a.Currency < b.Currency
	})
 if fold.age > 0 {
  out.LongOverdueDays = new(fold.age)
  aged := crmcontracts.CustomerPayments{Totals: []crmcontracts.CustomerPaymentTotal{}, Customers: []crmcontracts.CustomerPaymentCustomer{}}
  if fold.aged != nil { fold.aged.fill(&aged, connectionID) }
  out.LongOverdue = &crmcontracts.CustomerPaymentAgedGroup{InvoiceCount: aged.InvoiceCount, Totals: aged.Totals, Customers: aged.Customers}
 }
 if len(out.Customers) > 3 {
		out.Customers = out.Customers[:3]
	}
}
