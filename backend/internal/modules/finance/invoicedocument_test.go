// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"bytes"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Only a PDF within the size bound leaves the core. The claimed media type,
// the file's leading bytes and its size are each checked.
func TestOnlyABoundedPDFIsServed(t *testing.T) {
	pdf := []byte("%PDF-1.7 body")
	for name, document := range map[string]SourceDocument{
		"another media type":           {ContentType: "text/html", Body: pdf},
		"a PDF type over a page":       {ContentType: pdfMediaType, Body: []byte("<html>")},
		"a PDF larger than an invoice": {ContentType: pdfMediaType, Body: append(pdf, bytes.Repeat([]byte{' '}, maxInvoiceDocumentBytes)...)},
	} {
		if err := checkPDF(document); !errors.Is(err, apperrors.ErrProviderUnusable) {
			t.Errorf("%s: checkPDF = %v, want ErrProviderUnusable", name, err)
		}
	}
	if err := checkPDF(SourceDocument{ContentType: pdfMediaType, Body: pdf}); err != nil {
		t.Errorf("a small PDF was refused: %v", err)
	}
}

// A source's invoice number becomes the file name with nothing that could
// break the header or name a path.
func TestTheFileNameKeepsOnlySafeCharacters(t *testing.T) {
	number := "INV 7/\"x\"\r\n"
	got := documentFilename(documentTarget{externalID: "42", number: &number})
	if got != "invoice-INV_7_x_.pdf" {
		t.Errorf("filename = %q", got)
	}
	if got := documentFilename(documentTarget{externalID: "42"}); got != "invoice-42.pdf" {
		t.Errorf("an unnumbered invoice is named %q, want its source id", got)
	}
}
