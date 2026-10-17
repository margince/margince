// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The finance-source surface: a unit supplying read access to one accounting
// system, which the core mirrors into its finance tables. The core owns the
// connection, the customer mapping, the sync and every derived figure. The
// unit owns only how to reach its vendor, credentials included.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/pkg/extension/accounting"
)

// FinanceSource is one accounting system a unit can read.
//
// All three functions are required and all three only read. Each gets a fresh
// Runtime for the call. A credential the unit keeps in Secrets is read where it
// is spent, and a refreshed token is stored back the same way.
type FinanceSource struct {
	// Provider is the source's key, stored on the workspace's connection and
	// on every row the mirror writes. It follows Channel.Provider's grammar
	// (`^[a-z][a-z0-9_]*$`, 32 runes) and is unique across the composed set.
	Provider string

	// Check answers whether the source can be read now, without reading the
	// ledger. Nil means yes. The core calls it when an administrator connects
	// the source, so wrong credentials show at once rather than six hours later.
	Check FinanceCheck

	// Customers lists the source's whole customer directory.
	Customers FinanceCustomers

	// InvoicesFor answers one customer's invoices and payments.
	InvoicesFor FinanceInvoices
}

// FinanceCheck answers whether the source can be read now.
type FinanceCheck func(ctx context.Context, rt Runtime, conn accounting.Connection) error

// FinanceCustomers lists the source's customer directory.
type FinanceCustomers func(ctx context.Context, rt Runtime, conn accounting.Connection) ([]accounting.Customer, error)

// FinanceInvoices answers one customer's ledger.
type FinanceInvoices func(ctx context.Context, rt Runtime, conn accounting.Connection, externalCustomerID string) (accounting.Ledger, error)

// Validate enforces the provider grammar and that every function is present.
// A provider claimed by two units is a rule about the composed set, which the
// boot resolves.
func (s FinanceSource) Validate() error {
	if !providerGrammar.MatchString(s.Provider) || len([]rune(s.Provider)) > maxProviderLength {
		return fmt.Errorf("%w: finance source provider %q must start with a letter, hold only lower-case letters, digits and underscores, and be at most %d characters",
			ErrInvalid, s.Provider, maxProviderLength)
	}
	if s.Check == nil || s.Customers == nil || s.InvoicesFor == nil {
		return fmt.Errorf("%w: finance source %q must declare Check, Customers and InvoicesFor — the core calls each of them",
			ErrInvalid, s.Provider)
	}
	return nil
}
