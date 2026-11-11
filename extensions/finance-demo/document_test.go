// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package financedemo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
		if document.ContentType != "application/pdf" || !bytes.HasPrefix(document.Body, []byte("%PDF-1.4\n")) {
			t.Fatalf("%s: got %q, want a PDF", invoice.ExternalID, document.ContentType)
		}
		if !bytes.Contains(document.Body, []byte("(Demo invoice "+invoice.Number+")")) {
			t.Errorf("%s: the page does not name invoice %s", invoice.ExternalID, invoice.Number)
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

// A reader opens the file through its cross-reference table, so every offset
// it lists must land on the object it names.
func TestThePDFsCrossReferencePointsAtItsObjects(t *testing.T) {
	body := renderPDF([]string{"Total (EUR) 1.00", `a \ back`})
	entries := regexp.MustCompile(`(\d{10}) 00000 n `).FindAllSubmatch(body, -1)
	if len(entries) != 5 {
		t.Fatalf("the table lists %d objects, want 5", len(entries))
	}
	for i, entry := range entries {
		offset, err := strconv.Atoi(string(entry[1]))
		if err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("%d 0 obj", i+1); !bytes.HasPrefix(body[offset:], []byte(want)) {
			t.Errorf("entry %d points at %q, want %q", i+1, body[offset:offset+len(want)], want)
		}
	}
	if !bytes.Contains(body, []byte(`(Total \(EUR\) 1.00)`)) || !bytes.Contains(body, []byte(`(a \\ back)`)) {
		t.Errorf("the reserved characters are not escaped:\n%s", body)
	}
}
