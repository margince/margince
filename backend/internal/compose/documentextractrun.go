// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What one reading of one document does, from claiming the run record to
// closing it.
//
// The division of labour with documentextract.go is: that file owns the
// question put to the model and what may come back, this one owns the run — the
// claim, the lane, and the three outcomes a rep can be shown. Keeping the
// outcomes here is deliberate, because the difference between them is the
// product: "still reading", "read it and it states none of them", and "could
// not read it" must never collapse into one another (RD-AC-N-2).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/pdftext"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// maxDocumentBytes bounds what may be sent as an input part.
//
// It is derived from the wire, not chosen for roundness: inline bytes are
// base64 on every carrying adapter, which costs four bytes for every three, so
// this becomes ~10.7 MB of request body. Both adapters that carry documents
// today accept a request an order of magnitude under their vendors' ~20 MB
// ceiling at that size, leaving the prompt and the schema room that does not
// have to be counted per call.
//
// DOC-PARAM-4 lets a 25 MB file be ATTACHED, which is a different question from
// what one model call carries. A file above this is refused as a reading rather
// than truncated: half a scanned contract read confidently is worse than no
// reading at all, because nothing on the panel would say which half it saw.
const maxDocumentBytes = 8 << 20

// maxDocumentTextChars bounds the text lane, matching what a transcript reading
// addresses (activities.MaxReadableTranscriptChars) — the same question of how
// much prose one call can hold steadily, asked of the same models.
const maxDocumentTextChars = activities.MaxReadableTranscriptChars

// documentPDFMIME is the one media type the extraction lane below turns into
// text. Written here rather than imported from the ai module: this is the
// DOCUMENT side's question — what this reading can convert — and the wire side's
// list of what an adapter carries is a different question that happens to spell
// one entry the same way.
const documentPDFMIME = "application/pdf"

// documentTextMIMEs are the media types whose bytes ARE their text, so no
// parser stands between the file and the model.
//
// This is the lane the survey of other BYOK systems calls the
// provider-independent one, reached without any of their machinery: a file that
// is already text needs no OCR engine, no document-intelligence service and no
// vision-capable binding, and its quotes can be checked against the document
// verbatim (RD-AC-N-4) — which the bytes lane can never do.
//
// A PDF is NOT here and must not be added: its bytes are not its text, and the
// extraction lane reaches the same place by a route that says so.
var documentTextMIMEs = []string{
	"text/plain", "text/csv", "text/markdown", "text/html", "application/json", "message/rfc822",
}

// DocumentExtractor reads an attached document for the deal facts it states.
type DocumentExtractor struct {
	pool  *pgxpool.Pool
	brain documentCompleter
	pdf   *pdftext.Reader
	log   *slog.Logger
}

// documentCompleter is the completion seam for a lane whose input may be a
// document: it answers what it can CARRY as well as how to call it.
//
// The ordinary completer seam is not enough here, and the reason is the one the
// LiteLLM-style capability registry exists for. A lane that learns its binding
// is text-only by sending the bytes and being refused has already written a
// failed attempt into the operator's own call trace — for a configuration that
// is not failing, merely text-only. Asking first is what lets "this binding
// cannot read images" be reported as the plain fact it is.
type documentCompleter interface {
	completer
	// AttachmentMIMEs is what a caller may hand this lane, in
	// model.CarriesMIME's spelling. Empty means documents cannot go to it.
	AttachmentMIMEs() []string
}

// NewDocumentExtractor builds the engine over the pool and one model lane.
//
// The PDF reader is built here and kept, because it owns a compiled WebAssembly
// module: one per extractor, compiled on the first PDF this installation is
// asked to read and never again.
// The engine is not closed, and that is the whole lifecycle: it is compiled on
// the first PDF and lives as long as the worker that reads them, which is as
// long as the process. A Close here would need a shutdown seam the job runtime
// does not have, for a resource whose release coincides with exit.
func NewDocumentExtractor(pool *pgxpool.Pool, brain documentCompleter, log *slog.Logger) *DocumentExtractor {
	return &DocumentExtractor{pool: pool, brain: brain, pdf: pdftext.New(), log: log}
}

// documentReadStore is the slice of the activities store one reading drives.
// Named so the run can be tested against the real store or a double without
// either standing in for the whole module.
type documentReadStore interface {
	BeginExtractionRead(ctx context.Context, readID ids.UUID, reclaimAfter time.Duration) (activities.ExtractionRead, error)
	FinishExtractionRead(ctx context.Context, readID ids.UUID, outcome activities.ExtractionReadOutcome) error
	ReleaseExtractionRead(ctx context.Context, readID ids.UUID, claimedAt time.Time) error
	OpenAttachment(ctx context.Context, id ids.UUID) (crmcontracts.Attachment, io.ReadCloser, error)
}

// Read performs one reading: claim the run, put the document to the model on
// whichever lane its type and the binding allow, and close the run with what
// happened.
//
// It returns an error only for a fault the JOB should retry — the model lane
// being down, the database being unreachable. A reading that legitimately could
// not be used closes the run and returns nil, because retrying would ask the
// same question of the same document and get the same answer.
func (d *DocumentExtractor) Read(ctx context.Context, store documentReadStore, readID, attachmentID ids.UUID) error {
	claim, err := store.BeginExtractionRead(ctx, readID, activities.ExtractionReadLease)
	if err != nil {
		return err
	}
	// The claim's own start time travels with every write below, so each one is
	// scoped to THIS attempt: a lease that expired mid-call has already handed
	// the reading to somebody else, and this worker must not close or re-queue
	// what it no longer holds.
	claimedAt := claimStart(claim)

	src, detail, err := d.sourceFor(ctx, store, attachmentID)
	if err != nil {
		if terminal, msg := unreadableDocument(err); terminal {
			return d.fail(ctx, store, readID, claimedAt, msg)
		}
		// Anything else — the database unreachable, the object store down — is
		// the JOB's to retry. Closing the reading here would turn a blip into a
		// permanent verdict the rep has to notice and undo.
		return d.retryable(ctx, store, readID, claimedAt, err)
	}
	if detail != "" {
		// COULD NOT read it, which is not the same answer as read it and it
		// states none of them — and `done` is what the panel renders as the
		// second. A file this installation's model cannot carry, one with no
		// content type, one too large: each is a true "could not read", and each
		// is something an operator can change, so each also earns the retry
		// affordance only `failed` offers. No model call was made either way.
		return d.fail(ctx, store, readID, claimedAt, detail)
	}
	fields, err := d.ask(ctx, src)
	if err != nil {
		if errors.Is(err, errRefusedDocument) {
			d.log.WarnContext(ctx, "document reading refused",
				"attachment_extraction_id", readID, "attachment_id", attachmentID, "reason", err)
			return d.fail(ctx, store, readID, claimedAt,
				"the model's reading of this document could not be used; the document is unchanged and can be read again")
		}
		return d.retryable(ctx, store, readID, claimedAt, err)
	}
	return store.FinishExtractionRead(ctx, readID, activities.ExtractionReadOutcome{
		Status:    activities.ExtractionReadDone,
		Detail:    emptyReadingDetail(fields),
		Fields:    fields,
		ClaimedAt: claimedAt,
	})
}

// claimStart is the claimed row's own start time. It is set by the claim that
// just succeeded, so a zero here would mean the store returned a row it did not
// claim — worth failing loudly on rather than writing a zero timestamp into
// every subsequent CAS, where it would match nothing and look like a race.
func claimStart(claim activities.ExtractionRead) time.Time {
	if claim.StartedAt == nil {
		return time.Time{}
	}
	return *claim.StartedAt
}

// retryable hands the reading back before handing the job back.
//
// Returning the error alone would leave the row `running` with nobody working
// it: River retries inside the lease, the re-claim is refused as somebody
// else's, and the retry then reports success — a reading stranded live forever,
// rendering as "reading…" with nothing for a rep to press.
func (d *DocumentExtractor) retryable(
	ctx context.Context, store documentReadStore, readID ids.UUID, claimedAt time.Time, cause error,
) error {
	if err := store.ReleaseExtractionRead(ctx, readID, claimedAt); err != nil {
		d.log.WarnContext(ctx, "could not release a reading being retried",
			"attachment_extraction_id", readID, "error", err)
	}
	return cause
}

// emptyReadingDetail says why a completed reading offered nothing, and says
// nothing when it offered something.
//
// A reading that grounded no field is a correct and common answer — plenty of
// documents state none of the four — but an empty panel that does not explain
// itself reads as a broken feature, which is the one thing this whole shape
// exists to prevent.
func emptyReadingDetail(fields []extraction.ExtractedField) string {
	for _, f := range fields {
		if !f.Omitted {
			return ""
		}
	}
	return "this document states none of the deal fields clearly enough to offer one"
}

// sourceFor decides which lane this document takes, and reads it.
//
// It answers one of three ways, and the middle one is the point: a source to
// read, or a detail saying this installation cannot read this document, or an
// error. The detail is not a failure — the document is intact and the binding
// is working, they simply do not meet.
func (d *DocumentExtractor) sourceFor(
	ctx context.Context, store documentReadStore, attachmentID ids.UUID,
) (documentSource, string, error) {
	meta, body, err := store.OpenAttachment(ctx, attachmentID)
	if err != nil {
		return documentSource{}, "", err
	}
	defer func() {
		if err := body.Close(); err != nil {
			d.log.WarnContext(ctx, "closing document body", "attachment_id", attachmentID, "error", err)
		}
	}()
	// ContentType is optional on the row; a document that never declared one
	// takes neither lane, which is the honest answer — the alternative is
	// sniffing bytes here, a second content-type authority beside DOC-PARAM-9's.
	//
	// So the lane is decided by what INGRESS recorded, and what that means
	// depends on how the file arrived (ai-operational-spec §4.2): a captured
	// attachment carries the type sniffed from its bytes, with a disagreeing
	// sender claim recorded rather than obeyed, while a file uploaded through
	// the API carries its uploading principal's declared type unsniffed. Either
	// way carriage is not a content check — a binding's `input:` says what this
	// product will send, never what the bytes are.
	mime := ""
	if meta.ContentType != nil {
		mime = mediaTypeOf(*meta.ContentType)
	}
	// One byte past the bound is what distinguishes "exactly at the limit" from
	// "truncated to the limit"; without it a document of exactly maxDocumentBytes
	// and one of a gigabyte both arrive as a full buffer and look identical.
	bytes, err := io.ReadAll(io.LimitReader(body, maxDocumentBytes+1))
	if err != nil {
		return documentSource{}, "", fmt.Errorf("read document bytes: %w", err)
	}
	if len(bytes) > maxDocumentBytes {
		return documentSource{}, fmt.Sprintf(
			"this document is larger than the %d MB one reading carries; a reading of part of it could not say which part it saw",
			maxDocumentBytes>>20), nil
	}
	src, detail := d.laneFor(ctx, meta, mime, bytes)
	return src, detail, nil
}

// laneFor picks which of the lanes this document takes, or the reason none of
// them can have it. Split from sourceFor so the choice reads as one ordered list
// rather than trailing the reading of the bytes.
//
// The order is the contract. Native carriage is tried BEFORE extraction because
// a vendor that opens the document itself sees its layout — a table, a stamp, a
// signature block — where extraction sees only the characters; extraction is the
// answer for a wire that cannot be handed the file at all, not an improvement on
// one that can.
func (d *DocumentExtractor) laneFor(
	ctx context.Context, meta crmcontracts.Attachment, mime string, bytes []byte,
) (documentSource, string) {
	if model.CarriesMIME(documentTextMIMEs, mime) {
		return d.textSource(meta, bytes)
	}
	if model.CarriesMIME(d.brain.AttachmentMIMEs(), mime) {
		return documentSource{
			Part:     model.Attachment{MIME: mime, Bytes: bytes, Name: meta.Filename},
			Filename: meta.Filename,
		}, ""
	}
	if mime == documentPDFMIME {
		return d.extractedSource(ctx, meta, bytes)
	}
	if mime == "" {
		return documentSource{}, "this document declares no content type, so nothing can say how to read it"
	}
	return documentSource{}, fmt.Sprintf(
		"this installation's model cannot read a %s document; a file whose text can be read directly, or a model bound to carry documents, would be read",
		mime)
}

// extractedSource takes the lane that exists because no wire agrees about a PDF.
//
// A PDF rides a proprietary request-body extension on one gateway and nothing at
// all on a self-hosted endpoint, so this product never sends one there. It reads
// the text the document already carries and sends THAT, which every wire spells
// the same way — the model is handed characters, and was not asked to understand
// a file format.
func (d *DocumentExtractor) extractedSource(
	ctx context.Context, meta crmcontracts.Attachment, raw []byte,
) (documentSource, string) {
	text, err := d.pdf.Read(ctx, raw, maxDocumentTextChars)
	switch {
	case errors.Is(err, pdftext.ErrNoTextLayer):
		// A scan. Its pages are pictures, so there is no text to read and this
		// product has no engine that invents one. Named as the specific thing it
		// is, because "cannot read a PDF" would send an operator to their model
		// config when the answer is a different copy of the document.
		return documentSource{}, "this PDF holds no text, which is what a scanned or photographed document looks like; a PDF exported from the system that produced it can be read"
	case err != nil:
		return documentSource{}, "this PDF could not be opened to read its text"
	}
	// Held to the SAME bound the text lane holds, by the same words. Extract
	// answers one rune past the budget precisely so this can tell a document
	// that fits from one that was cut off, and a reading of the first 60,000
	// characters of a contract reported as `done` is the outcome maxDocumentBytes'
	// own comment refuses: nothing on the panel would say which part it saw.
	if utf8.RuneCountInString(text) > maxDocumentTextChars {
		return documentSource{}, fmt.Sprintf(
			"this document is longer than the %d characters one reading addresses",
			maxDocumentTextChars)
	}
	return documentSource{Text: text, Filename: meta.Filename, ExtractedFrom: documentPDFMIME}, ""
}

// mediaTypeOf reduces a stored Content-Type to the media type alone.
//
// What ingress recorded is a HEADER, and a header carries parameters: an upload
// declaring `application/pdf; charset=binary` is declaring a PDF, and every lane
// below compares against bare media types. Without this the parameter decides
// the lane — the same document read or refused depending on which client sent
// it — which is the sort of difference nobody can see from the panel.
//
// Not mime.ParseMediaType: that returns an error for a malformed header, and a
// caller-chosen string being malformed is not an outcome worth distinguishing
// here. Everything up to the first `;`, folded, is the whole answer.
func mediaTypeOf(contentType string) string {
	media, _, _ := strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(media))
}

// textSource takes the text lane, where the document's bytes are its text.
func (d *DocumentExtractor) textSource(meta crmcontracts.Attachment, raw []byte) (documentSource, string) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return documentSource{}, "this document carries no text to read"
	}
	if utf8.RuneCountInString(text) > maxDocumentTextChars {
		return documentSource{}, fmt.Sprintf(
			"this document is %d characters, and one reading addresses at most %d",
			len(text), maxDocumentTextChars)
	}
	return documentSource{Text: text, Filename: meta.Filename}, ""
}

// ask puts one document to the model and returns what it may act on.
//
// The validator runs on the shipping path, not only in the certification lane.
// A site whose corpus grades a validator production never calls is certifying a
// standard the product does not hold itself to — the reply that would have been
// refused in the lane reaches a deal instead.
func (d *DocumentExtractor) ask(ctx context.Context, src documentSource) ([]extraction.ExtractedField, error) {
	req := documentExtractRequest(src)
	validate := documentShapeValid(src)
	resp, err := ai.Ask(ctx, d.brain, req, validate)
	if err != nil {
		if errors.Is(err, model.ErrAttachmentUnsupported) {
			// The binding declared it carries this type and then refused it on
			// the wire. That is the two halves of one declaration disagreeing,
			// which is a configuration fault rather than a fault of this
			// document — so it is refused, not retried.
			return nil, fmt.Errorf("%w: the model refused a document type its binding declares it carries: %w",
				errRefusedDocument, err)
		}
		if errors.Is(err, ai.ErrOutputRejected) {
			return nil, fmt.Errorf("%w: %w", errRefusedDocument, err)
		}
		return nil, err
	}
	// Validated again even when CompleteValidated already ran it: a bare
	// completer (the offline fake, a role wired without the structured lane)
	// does not, and this is the only floor those paths have.
	if err := validate(resp.Text); err != nil {
		return nil, fmt.Errorf("%w: %w", errRefusedDocument, err)
	}
	return readDocumentFields(resp.Text)
}

// unreadableDocument separates a refusal a rep can act on from a fault the job
// should retry, and answers with the message the rep is shown.
//
// Only the typed refusals reach status_detail. A raw err.Error() there would
// put a driver string ("failed to connect to host=…") in front of a rep on the
// one field this feature exists to make readable, and would settle a transient
// blip as a permanent verdict.
func unreadableDocument(err error) (terminal bool, detail string) {
	if errors.Is(err, apperrors.ErrNotFound) {
		return true, "this document is no longer available to read"
	}
	if errors.Is(err, activities.ErrBlobstoreUnconfigured) {
		return true, "this installation stores no document bytes, so there is nothing to read"
	}
	return false, ""
}

// fail closes the run with a reason a rep can act on. A failure to record the
// failure is returned, so a run cannot be left claimed and silent.
func (d *DocumentExtractor) fail(
	ctx context.Context, store documentReadStore, readID ids.UUID, claimedAt time.Time, detail string,
) error {
	return store.FinishExtractionRead(ctx, readID, activities.ExtractionReadOutcome{
		Status: activities.ExtractionReadFailed, Detail: detail, ClaimedAt: claimedAt,
	})
}
