// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading the documents on the message a thread verdict judges.
//
// A covering note says "please find attached the invoice" whatever is
// attached. A verdict that reads only the note opens a salary sheet to the
// workspace, so it reads a capped excerpt of each document. That happens here,
// after the claim and before the model call, so the capture sink does no
// extra work.
//
// The excerpt is never stored. A cached copy would be the file's text at rest
// a second time, out of reach of the private-thread strip and of erasure.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/doctext"
	"github.com/margince/margince/backend/internal/platform/pdftext"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

const (
	// confidentialityDocumentBytes is the largest file the verdict reads. A
	// larger one is marked unreadable without being fetched.
	confidentialityDocumentBytes = 4 << 20
	// confidentialityDocumentChars is each document's share of the prompt, the
	// same order as the body's.
	confidentialityDocumentChars = 1500
	// confidentialityCVChars is how much of a document the CV rule reads. A
	// CV's headings often start past the prompt's share. The staff-cost rule
	// reads only the excerpt: over more text, decks with a team page match it.
	confidentialityCVChars = 6000
	// confidentialityDocumentTimeout bounds one file's parse. A file that
	// needs longer is marked unreadable, because it would need longer again.
	confidentialityDocumentTimeout = 10 * time.Second
	// confidentialityPassDocumentBytes bounds what one pass fetches from the
	// object store, so the pass cap is not the only limit on its load.
	confidentialityPassDocumentBytes = 64 << 20
	// confidentialityDocumentMargin is how close to the job deadline a pass
	// stops starting reads, so the threads it cannot finish are released.
	confidentialityDocumentMargin = time.Minute
	// confidentialityDocumentSlots is how many files the process parses at
	// once across every workspace's pass.
	confidentialityDocumentSlots = 2
)

// errDocumentsDeferred is a thread whose documents cannot be read now. The
// object store or database is down, or the pass is out of bytes or time. The
// thread is refunded and waits. Judged without its
// documents, it could open the file the excerpt exists to hold.
var errDocumentsDeferred = errors.New("confidentiality: the thread's documents cannot be read now")

// confidentialityDeferred is the engine's "not now, and not this thread's
// fault": the AI lane's deferrals and its own.
func confidentialityDeferred(err error) bool {
	return ai.IsDeferral(err) || errors.Is(err, errDocumentsDeferred)
}

// processPDFReader is the one PDF engine of the process. pdftext compiles a
// 5 MB module per Reader, and its pool bounds parallel parses, so every PDF
// reader in compose shares it.
var processPDFReader = pdftext.New()

// attachmentOpener is the slice of the activities store the verdict reads
// files through: the gated reader that refuses an archived or withheld file.
type attachmentOpener interface {
	OpenAttachment(ctx context.Context, id ids.UUID) (crmcontracts.Attachment, io.ReadCloser, error)
}

// threadDocuments reads the documents of claimed threads for one engine.
type threadDocuments struct {
	files attachmentOpener
	pdf   *pdftext.Reader
	slots chan struct{}
}

func newThreadDocuments(pool *pgxpool.Pool, blob blobstore.Store) *threadDocuments {
	return &threadDocuments{
		files: activities.NewStore(InstallationDB(pool)).WithBlobstore(blob),
		pdf:   processPDFReader,
		slots: make(chan struct{}, confidentialityDocumentSlots),
	}
}

// WithObjectStore lets the engine read the documents a verdict judges. Without
// one every document is marked unreadable, which the prompt says.
func (e *ConfidentialityVerdictEngine) WithObjectStore(blob blobstore.Store) *ConfidentialityVerdictEngine {
	out := *e
	out.documents = newThreadDocuments(e.pool, blob)
	return &out
}

// isPersonnelWithoutAModel says whether the thread is personnel by a rule. The
// body or a document lists staff with amounts, or a document is a CV by its
// text or name. Or a file the verdict did not read, an image or a scan among
// them, is named as an invoice, cost sheet or CV. A model would read a staff
// invoice as ordinary trade and a CV as an application.
func isPersonnelWithoutAModel(row capture.PendingThread) bool {
	if capture.ListsStaffCosts(row.Body) {
		return true
	}
	for _, doc := range row.Documents {
		if doc.Personnel || capture.NamedAsCV(doc.Filename) {
			return true
		}
	}
	return hasUnreadNamedFile(row, capture.NamedAsPersonnel)
}

func hasUnreadNamedFile(row capture.PendingThread, named func(string) bool) bool {
	// Names are counted, not keyed: a readable file must not vouch for an
	// unread one that shares its name.
	readByName := map[string]int{}
	for _, doc := range row.Documents {
		if doc.Unreadable && named(doc.Filename) {
			return true
		}
		if !doc.Unreadable {
			readByName[doc.Filename]++
		}
	}
	for _, name := range row.Attachments {
		if readByName[name] > 0 {
			readByName[name]--
			continue
		}
		if named(name) {
			return true
		}
	}
	return false
}

// documentPass is what one verdict pass may still fetch.
type documentPass struct {
	bytesLeft int64
}

func newDocumentPass() *documentPass {
	return &documentPass{bytesLeft: confidentialityPassDocumentBytes}
}

// read returns row with an excerpt or an unreadable mark on each document. A
// file the store no longer serves is marked, not dropped. A lost object must
// not leave the verdict believing it saw every file.
func (d *threadDocuments) read(
	ctx context.Context, row capture.PendingThread, pass *documentPass,
) (capture.PendingThread, error) {
	if len(row.Documents) == 0 {
		return row, nil
	}
	docs := slices.Clone(row.Documents)
	for i := range docs {
		if err := d.readOne(ctx, &docs[i], pass); err != nil {
			return row, err
		}
	}
	row.Documents = docs
	return row, nil
}

// readOne fills one document's excerpt, or marks it unreadable. It answers an
// error only for a cause the thread is not to blame for.
func (d *threadDocuments) readOne(ctx context.Context, doc *capture.ThreadDocument, pass *documentPass) error {
	if doc.ByteSize > confidentialityDocumentBytes || capture.IsThreadImage(doc.Filename, doc.ContentType) {
		doc.Unreadable = true
		return nil
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < confidentialityDocumentMargin {
		return fmt.Errorf("%w: the pass is out of time", errDocumentsDeferred)
	}
	if doc.ByteSize > pass.bytesLeft {
		return fmt.Errorf("%w: the pass spent its read budget", errDocumentsDeferred)
	}
	select {
	case d.slots <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", errDocumentsDeferred, ctx.Err())
	}
	defer func() { <-d.slots }()
	raw, mediaType, err := d.fetch(ctx, doc.AttachmentID)
	switch {
	case errors.Is(err, activities.ErrBlobstoreUnconfigured), errors.Is(err, errDocumentTooLarge),
		errors.Is(err, apperrors.ErrNotFound):
		doc.Unreadable = true
		return nil
	case err != nil:
		return fmt.Errorf("%w: %w", errDocumentsDeferred, err)
	}
	pass.bytesLeft -= int64(len(raw))
	return d.excerpt(ctx, doc, raw, mediaType)
}

// errDocumentTooLarge is a file whose bytes outgrew the size its row states.
var errDocumentTooLarge = errors.New("confidentiality: the document is larger than the verdict reads")

// fetch reads one file's bytes, at most one past the cap so a file that grew
// past its stated size is caught rather than cut.
func (d *threadDocuments) fetch(ctx context.Context, id ids.UUID) ([]byte, string, error) {
	meta, body, err := d.files.OpenAttachment(ctx, id)
	if err != nil {
		return nil, "", err
	}
	raw, readErr := io.ReadAll(io.LimitReader(body, confidentialityDocumentBytes+1))
	if closeErr := body.Close(); readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return nil, "", readErr
	}
	if len(raw) > confidentialityDocumentBytes {
		return nil, "", errDocumentTooLarge
	}
	mediaType := ""
	if meta.ContentType != nil {
		mediaType = mediaTypeOf(*meta.ContentType)
	}
	return raw, mediaType, nil
}

// excerpt parses the bytes under the file's own time bound. A parse failure is
// the file's and marks it unreadable; the caller's own cancellation is not, and
// defers the thread.
func (d *threadDocuments) excerpt(ctx context.Context, doc *capture.ThreadDocument, raw []byte, mediaType string) error {
	if capture.IsThreadImage(doc.Filename, mediaType) {
		doc.Unreadable = true
		return nil
	}
	fileCtx, cancel := context.WithTimeout(ctx, confidentialityDocumentTimeout)
	defer cancel()
	text, truncated, err := d.parse(fileCtx, raw, mediaType, confidentialityCVChars)
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", errDocumentsDeferred, ctx.Err())
	}
	if err != nil {
		doc.Unreadable = true
		return nil
	}
	read := text
	if utf8.RuneCountInString(text) > confidentialityDocumentChars {
		text, truncated = textcut.Runes(text, confidentialityDocumentChars), true
	}
	doc.Excerpt, doc.Truncated = text, truncated
	doc.Personnel = documentReadsAsPersonnel(read, text)
	return nil
}

// documentReadsAsPersonnel is how the read step marks a document by its text.
// The CV rule reads more of the file than the excerpt keeps.
func documentReadsAsPersonnel(read, excerpt string) bool {
	return capture.DescribesACareer(read) || capture.ListsStaffCosts(excerpt)
}

// parse dispatches a PDF to pdftext and everything else to doctext.
func (d *threadDocuments) parse(ctx context.Context, raw []byte, mediaType string, maxChars int) (string, bool, error) {
	if mediaType == documentPDFMIME || bytes.HasPrefix(raw, []byte("%PDF-")) {
		text, err := d.pdf.Read(ctx, raw, maxChars)
		if err != nil {
			return "", false, err
		}
		if utf8.RuneCountInString(text) > maxChars {
			return textcut.Runes(text, maxChars), true, nil
		}
		return text, false, nil
	}
	excerpt, err := doctext.Read(ctx, raw, maxChars)
	return excerpt.Text, excerpt.Truncated, err
}
