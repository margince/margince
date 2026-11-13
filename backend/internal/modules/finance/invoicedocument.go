// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
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

// invoiceDocumentsPerMinute is how many documents one caller may fetch in a
// minute. Each one is a live call to the vendor, whose quota the whole
// installation shares, and a reader opening invoices by hand needs far fewer.
const invoiceDocumentsPerMinute = 20

// invoiceDocumentFetches meters document fetches per caller. FailClosed: an
// unmetered window can spend the vendor quota for the day, and that cannot be
// taken back.
var invoiceDocumentFetches = ratelimit.New("finance-invoice-document", ratelimit.FailClosed, invoiceDocumentsPerMinute, time.Minute)

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
// An invoice the caller cannot see answers the same 404 as a missing one. Only
// a PDF is ever handed back.
func (s *Store) InvoiceDocumentFor(ctx context.Context, invoiceID ids.UUID) (InvoiceDocument, error) {
	if err := auth.Require(ctx, "finance", principal.ActionRead); err != nil {
		return InvoiceDocument{}, err
	}
	var target documentTarget
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var readErr error
		target, readErr = readDocumentTarget(ctx, tx, invoiceID)
		return readErr
	})
	if err != nil {
		return InvoiceDocument{}, err
	}
	// Counted only once the invoice is found and visible, so a miss spends
	// nothing of the caller's budget.
	actor, ok := principal.Actor(ctx)
	if !ok {
		return InvoiceDocument{}, apperrors.ErrPermissionDenied
	}
	if !invoiceDocumentFetches.Allow(actor.ID) {
		return InvoiceDocument{}, fmt.Errorf("%w: at most %d invoice documents a minute; try again in %d seconds",
			apperrors.ErrBudgetExceeded, invoiceDocumentsPerMinute, invoiceDocumentFetches.RetryAfterSeconds())
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

// readDocumentTarget reads the invoice on the workspace's live connection, and
// only for a caller who can see its live company. An invoice left behind by an
// earlier connection is not found: its source is no longer the one read.
// Archiving a company or unlinking its customers leaves its invoices in the
// mirror. The card then lists none of them, so their documents are not found.
func readDocumentTarget(ctx context.Context, tx pgx.Tx, invoiceID ids.UUID) (documentTarget, error) {
	conn, connected, err := readConnection(ctx, tx)
	if err != nil {
		return documentTarget{}, err
	}
	if !connected {
		return documentTarget{}, apperrors.ErrNotFound
	}
	target := documentTarget{
		provider: conn.provider,
		conn:     SourceConnection{ID: conn.id.String(), ConnectedAt: conn.createdAt},
	}
	err = tx.QueryRow(ctx, `
		SELECT company_id, external_id, number
		  FROM finance_invoice
		 WHERE id = $1 AND connection_id = $2 AND archived_at IS NULL`, invoiceID, conn.id).
		Scan(&target.companyID, &target.externalID, &target.number)
	if errors.Is(err, pgx.ErrNoRows) {
		return documentTarget{}, apperrors.ErrNotFound
	}
	if err != nil {
		return documentTarget{}, fmt.Errorf("read the invoice: %w", err)
	}
	if err := auth.EnsureVisibleLive(ctx, tx, "company", target.companyID); err != nil {
		return documentTarget{}, err
	}
	linked, err := companyIsLinked(ctx, tx, ids.CompanyID{UUID: target.companyID}, conn.id)
	if err != nil {
		return documentTarget{}, err
	}
	if !linked {
		return documentTarget{}, apperrors.ErrNotFound
	}
	return target, nil
}

// checkPDF refuses a document that is not a PDF within the size bound. The
// media type alone is the source's claim; the leading bytes are the file's.
// A media type matches in any case and with parameters, as HTTP spells it.
func checkPDF(document SourceDocument) error {
	mediaType, _, err := mime.ParseMediaType(document.ContentType)
	switch {
	case err != nil || mediaType != pdfMediaType:
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
		ContentType: pdfMediaType, Filename: document.Filename, Inline: true, Size: int64(len(document.Body)),
		Body: io.NopCloser(bytes.NewReader(document.Body)),
	}, "finance invoice pdf "+id.String())
}
