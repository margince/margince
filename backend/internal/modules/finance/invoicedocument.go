// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maxInvoiceDocumentBytes bounds what one invoice document may hold. A source
// answering more is answering something other than an invoice.
const maxInvoiceDocumentBytes = 20 << 20

// pdfMediaType is the one kind of document the core serves. Anything else a
// source sends could be rendered by the browser as a page of our origin.
const pdfMediaType = "application/pdf"

// unsafeFileRune is what a source's invoice number may not carry into a file name.
var unsafeFileRune = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// InvoiceDocument is one mirrored invoice's own document, read from the
// connected source for this request and never stored.
type InvoiceDocument struct {
	Body     []byte
	Filename string
}

// documentTarget is what reading a document needs from the mirror: which
// source to ask, and what the source calls the invoice.
type documentTarget struct {
	companyID  ids.UUID
	externalID string
	number     *string
	provider   string
	conn       SourceConnection
}

// InvoiceDocumentFor fetches one mirrored invoice's document from the source.
// It answers 404 for an invoice the caller cannot see exactly as for one that
// does not exist, and only ever hands back a PDF.
func (s *Store) InvoiceDocumentFor(ctx context.Context, invoiceID ids.UUID) (InvoiceDocument, error) {
	if err := auth.Require(ctx, "finance", principal.ActionRead); err != nil {
		return InvoiceDocument{}, err
	}
	var target documentTarget
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var readErr error
		target, readErr = readDocumentTarget(ctx, tx, invoiceID)
		if readErr != nil {
			return readErr
		}
		return auth.EnsureVisible(ctx, tx, "company", target.companyID)
	})
	if err != nil {
		return InvoiceDocument{}, err
	}
	// The source is asked outside the transaction, so a slow vendor holds no
	// database connection.
	provider, ok := s.open(target.provider, target.conn)
	if !ok {
		return InvoiceDocument{}, fmt.Errorf("%w: no installed unit reads %s", apperrors.ErrUnsupportedBySoR, target.provider)
	}
	document, err := provider.InvoiceDocument(ctx, target.externalID)
	if errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		return InvoiceDocument{}, err
	}
	if err != nil {
		return InvoiceDocument{}, fmt.Errorf("%w: reading the invoice document: %w", apperrors.ErrProviderUnusable, err)
	}
	if err := checkPDF(document); err != nil {
		return InvoiceDocument{}, err
	}
	return InvoiceDocument{Body: document.Body, Filename: documentFilename(target)}, nil
}

// servesDocuments answers whether the named source serves invoice documents,
// which decides whether the card offers to open one.
func (s *Store) servesDocuments(provider string) bool {
	if s.sources == nil {
		return false
	}
	for _, option := range s.sources.Providers() {
		if option.Provider == provider {
			return option.Documents
		}
	}
	return false
}

// readDocumentTarget reads the invoice on the workspace's live connection. An
// invoice left behind by an earlier connection is not found: the source that
// issued it is no longer the one this installation reads.
func readDocumentTarget(ctx context.Context, tx pgx.Tx, invoiceID ids.UUID) (documentTarget, error) {
	var target documentTarget
	var connID ids.UUID
	var connectedAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT i.company_id, i.external_id, i.number, c.provider, c.id, c.created_at
		  FROM finance_invoice i
		  JOIN finance_connection c ON c.id = i.connection_id
		 WHERE i.id = $1 AND i.archived_at IS NULL
		   AND c.archived_at IS NULL AND c.status <> 'disconnected'
		   AND c.id = (SELECT id FROM finance_connection
		                WHERE archived_at IS NULL AND status <> 'disconnected'
		                ORDER BY created_at DESC LIMIT 1)`, invoiceID).
		Scan(&target.companyID, &target.externalID, &target.number, &target.provider, &connID, &connectedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return documentTarget{}, apperrors.ErrNotFound
	}
	if err != nil {
		return documentTarget{}, fmt.Errorf("read the invoice: %w", err)
	}
	target.conn = SourceConnection{ID: connID.String(), ConnectedAt: connectedAt}
	return target, nil
}

// checkPDF refuses a document that is not a PDF within the size bound. The
// media type alone is the source's claim; the leading bytes are the file's.
func checkPDF(document SourceDocument) error {
	switch {
	case document.ContentType != pdfMediaType:
		return fmt.Errorf("%w: the source sent %q, not a PDF", apperrors.ErrProviderUnusable, document.ContentType)
	case len(document.Body) > maxInvoiceDocumentBytes:
		return fmt.Errorf("%w: the source sent %d bytes, more than an invoice holds", apperrors.ErrProviderUnusable, len(document.Body))
	case !bytes.HasPrefix(document.Body, []byte("%PDF-")):
		return fmt.Errorf("%w: the source's document is not a PDF file", apperrors.ErrProviderUnusable)
	}
	return nil
}

// documentFilename names the file after the invoice's own number, falling
// back to its source id when the source issued none.
func documentFilename(target documentTarget) string {
	name := target.externalID
	if target.number != nil && *target.number != "" {
		name = *target.number
	}
	return "invoice-" + unsafeFileRune.ReplaceAllString(name, "_") + ".pdf"
}

// DownloadFinanceInvoicePdf implements GET /finance/invoices/{id}/pdf.
func (h Handlers) DownloadFinanceInvoicePdf(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	document, err := h.store.InvoiceDocumentFor(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// An invoice is read under the caller's own grant, so no shared cache may keep it.
	w.Header().Set("Cache-Control", "no-store")
	httperr.StreamObject(w, r, httperr.StreamedObject{
		Download: httperr.Download{ContentType: pdfMediaType, Filename: document.Filename, Inline: true, Size: int64(len(document.Body))},
		Body:     io.NopCloser(bytes.NewReader(document.Body)),
	}, "finance invoice pdf "+id.String())
}
