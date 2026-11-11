// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package financedemo

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/accounting"
)

// errNoSuchInvoice is an invoice id this source never generated.
const errNoSuchInvoice documentError = "finance-demo: no generated invoice has that id"

// documentError is one of this unit's own refusals. A string constant, since
// the generator refuses a package-level value built by a call.
type documentError string

func (e documentError) Error() string { return string(e) }

// invoiceDocument renders a generated invoice as a one-page PDF. It names
// itself demo data, as every figure this source answers does.
func invoiceDocument(_ context.Context, _ extension.Runtime, conn accounting.Connection, externalInvoiceID string) (accounting.Document, error) {
	customerID, ok := customerOf(externalInvoiceID)
	if !ok {
		return accounting.Document{}, errNoSuchInvoice
	}
	ledger, err := generate(conn, customerID)
	if err != nil {
		return accounting.Document{}, err
	}
	for _, invoice := range ledger.Invoices {
		if invoice.ExternalID != externalInvoiceID {
			continue
		}
		body, err := renderPDF(documentLines(invoice))
		if err != nil {
			return accounting.Document{}, err
		}
		return accounting.Document{ContentType: "application/pdf", Body: body}, nil
	}
	return accounting.Document{}, errNoSuchInvoice
}

// customerOf reads the customer back out of a generated invoice id, which
// generate spells as the customer id followed by -INV- or -CN-.
func customerOf(externalInvoiceID string) (string, bool) {
	for _, marker := range []string{"-INV-", "-CN-"} {
		if customer, _, found := strings.Cut(externalInvoiceID, marker); found {
			return customer, true
		}
	}
	return "", false
}

// documentLines is what the page says about one invoice.
func documentLines(invoice accounting.Invoice) []string {
	lines := []string{
		"Demo invoice " + invoice.Number,
		"Issued " + invoice.IssuedOn.Format("2006-01-02"),
	}
	if invoice.DueOn != nil {
		lines = append(lines, "Due "+invoice.DueOn.Format("2006-01-02"))
	}
	return append(lines,
		fmt.Sprintf("Total %s %d.%02d", invoice.Currency, invoice.GrossMinor/100, abs(invoice.GrossMinor%100)),
		"Generated demonstration data, not a real invoice.",
	)
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// renderPDF writes lines of text onto one A4 page.
func renderPDF(lines []string) ([]byte, error) {
	page := fpdf.New("P", "mm", "A4", "")
	page.AddPage()
	page.SetFont("Helvetica", "", 12)
	for _, line := range lines {
		page.Cell(0, 8, line)
		page.Ln(8)
	}
	var out bytes.Buffer
	if err := page.Output(&out); err != nil {
		return nil, fmt.Errorf("finance-demo: rendering the invoice page: %w", err)
	}
	return out.Bytes(), nil
}
