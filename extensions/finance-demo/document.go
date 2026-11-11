// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package financedemo

import (
	"bytes"
	"context"
	"fmt"
	"strings"

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
		if invoice.ExternalID == externalInvoiceID {
			return accounting.Document{ContentType: "application/pdf", Body: renderPDF(documentLines(invoice))}, nil
		}
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

// renderPDF writes the smallest valid PDF holding lines of text: a catalog,
// one A4 page, its content stream and a standard font, with the byte offsets
// the cross-reference table needs.
func renderPDF(lines []string) []byte {
	var content strings.Builder
	content.WriteString("BT /F1 14 Tf 72 770 Td 20 TL\n")
	for _, line := range lines {
		fmt.Fprintf(&content, "(%s) Tj T*\n", escapePDF(line))
	}
	content.WriteString("ET")
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", content.Len(), content.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	table := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, table)
	return out.Bytes()
}

// escapePDF escapes the three characters a PDF string literal reserves.
func escapePDF(text string) string {
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(text)
}
