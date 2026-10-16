// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package accounting carries the records a finance source hands the core's
// finance mirror: an accounting system's customers, invoices and payments, as
// that system states them.
//
// One rule shapes every type here: a source reports what it said, and the
// mirror derives everything else. Status is the sharp case: "overdue" depends
// on today. A source that returned it would change an untouched invoice every
// morning, and every sync would rewrite rows that did not change.
//
//margince:extension-surface
package accounting

import "time"

// Connection is the workspace's link to one accounting source, as the core
// recorded it. A source keys anything it derives per installation on ID.
type Connection struct {
	ID          string
	ConnectedAt time.Time
}

// Customer is one entry in the source's customer directory: what an
// administrator maps onto a company. The core never matches one
// automatically, because a wrong guess puts money on the wrong account.
type Customer struct {
	ExternalID  string
	DisplayName string
	// UpdatedAt is the source's own modification stamp, when it keeps one. It
	// is reported, never used to detect change: a source that touches every
	// row nightly would otherwise look like one that changed every row.
	UpdatedAt *time.Time
}

// Ledger is one customer's invoices and the payments received from them.
type Ledger struct {
	Invoices []Invoice
	Payments []Payment
}

// Invoice is an invoice as the source states it. It has no status field: the
// mirror derives status on read from the fields below.
type Invoice struct {
	ExternalID string
	Number     string
	IssuedOn   time.Time
	DueOn      *time.Time
	Currency   string
	NetMinor   int64
	TaxMinor   int64
	GrossMinor int64
	// PaidMinor is what the source says it received against this invoice. The
	// open balance is derived from it, not stored twice.
	PaidMinor int64
	// FullyPaidAt is set only once the source considers the invoice settled.
	// A partly paid invoice has no settlement date and so no punctuality.
	FullyPaidAt *time.Time
	Disputed    bool
	Void        bool
	// CreditsExternalID names the invoice a credit note reduces. Empty on an
	// ordinary invoice.
	CreditsExternalID string
	UpdatedAt         *time.Time
}

// Payment is one payment received.
type Payment struct {
	ExternalID string
	// InvoiceExternalID is empty for a payment the source has not applied to
	// an invoice. The mirror keeps it unapplied rather than guess the invoice.
	InvoiceExternalID string
	PaidAt            time.Time
	Currency          string
	AmountMinor       int64
	UpdatedAt         *time.Time
}
