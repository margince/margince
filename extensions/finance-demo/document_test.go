// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package financedemo

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension/accounting"
)

// Every invoice and credit note the ledger lists opens as a PDF naming it.
func TestEveryGeneratedInvoiceOpensAsItsOwnPDF(t *testing.T) {
	conn := accounting.Connection{ID: "conn-1", ConnectedAt: epoch}
	for _, invoice := range ledgerAt(t, conn.ID, epoch, "DEMO-0007").Invoices {
		document, err := invoiceDocument(context.Background(), nil, conn, invoice.ExternalID)
		if err != nil {
			t.Fatalf("%s: %v", invoice.ExternalID, err)
		}
		if document.ContentType != "application/pdf" || !bytes.HasPrefix(document.Body, []byte("%PDF-")) {
			t.Fatalf("%s: got %q, want a PDF", invoice.ExternalID, document.ContentType)
		}
	}
}

func TestAnInvoiceIdNeverGeneratedIsRefused(t *testing.T) {
	conn := accounting.Connection{ID: "conn-1", ConnectedAt: epoch}
	for _, id := range []string{"DEMO-0007-INV-9999", "somebody-else's", ""} {
		if _, err := invoiceDocument(context.Background(), nil, conn, id); !errors.Is(err, errNoSuchInvoice) {
			t.Errorf("%q answered %v, want errNoSuchInvoice", id, err)
		}
	}
}

// The page names the invoice it is, what it is worth, and that it is demo data.
func TestThePageNamesTheInvoiceAndSaysItIsADemo(t *testing.T) {
	invoice := ledgerAt(t, "conn-1", epoch, "DEMO-0007").Invoices[0]
	lines := documentLines(invoice)
	page := strings.Join(lines, "\n")
	for _, want := range []string{"Demo invoice " + invoice.Number, invoice.Currency, "not a real invoice"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q:\n%s", want, page)
		}
	}
}
