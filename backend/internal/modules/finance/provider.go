// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// The seam an accounting source is read through. The core ships no source of
// its own. Every one, the demo included, is an extension unit's FinanceSource,
// adapted onto this interface where a connection is resolved.

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension/accounting"
)

// Provider reads one accounting source. It answers records; it never writes
// them, never decides an invoice's status, and never sees the database.
type Provider interface {
	// Name is the provider's own identifier, stored on every mirrored row so
	// a reader can tell which source a figure came from.
	Name() string
	// Check answers whether the source can be read now; nil means yes.
	Check(ctx context.Context) error
	// Customers lists the source's own customer directory.
	Customers(ctx context.Context) ([]SourceCustomer, error)
	// InvoicesFor answers one customer's invoices and payments.
	InvoicesFor(ctx context.Context, externalCustomerID string) (SourceLedger, error)
}

// Sources resolves the providers this build can read. The composition
// implements it over the composed units; the sweep and the connect flow both
// resolve providers through it.
type Sources interface {
	// Providers lists every source on offer, sorted by provider.
	Providers() []SourceOption
	// Open binds one provider to a connection; false when no composed unit
	// supplies that name.
	Open(provider string, conn SourceConnection) (Provider, bool)
}

// SourceOption is one source on offer: its key, and what the unit supplying it
// says it is for.
type SourceOption struct {
	Provider    string
	Description string
}

// The source records are the published accounting types, aliased so a unit's
// record and the mirror's input are one type with one definition.
type (
	// SourceConnection is accounting.Connection.
	SourceConnection = accounting.Connection
	// SourceCustomer is accounting.Customer.
	SourceCustomer = accounting.Customer
	// SourceLedger is accounting.Ledger.
	SourceLedger = accounting.Ledger
	// SourceInvoice is accounting.Invoice.
	SourceInvoice = accounting.Invoice
	// SourcePayment is accounting.Payment.
	SourcePayment = accounting.Payment
)
