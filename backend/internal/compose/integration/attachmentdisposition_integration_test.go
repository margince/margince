// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The one line between "a stranger sent us a file" and stored XSS.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A downloaded attachment is SAVED, never rendered — including one whose bytes
// are markup. The property lives on the SHARED download path. The file is
// captured rather than uploaded, because an upload refuses HTML outright and a
// sender's mail does not.
//
// An attachment's bytes are sender-controlled, its content type is sniffed from
// those same bytes, and a text/html file served inline would execute in the
// viewer's session against this origin. The property is currently free —
// DownloadAttachment does not ask for Inline — and this test exists so that
// adding an inline preview has to confront the fact that a captured channel file
// comes from an untrusted sender rather than trip over it.
func TestDownloadedAttachmentIsSavedNotRendered(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	h := activities.NewHandlers(e.DB()).WithUploadLimit(uploadCeiling).WithBlobstore(blob)
	ctx := e.Admin()
	contact := e.SeedContact(t, "Untrusted Sender", &e.Rep1)
	owner := OwnerConn(t)
	email := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Your invoice', now(), 'imap', 'connector:imap')`)
	LinkActivity(t, owner, email, "contact", contact)

	// Markup, because that is the shape that would execute: a benign PDF proves
	// the header is set, not that the dangerous case is covered by it.
	capture := e.ConnectorOwnerCtx(e.AdminUser, "imap")
	store := activities.NewStore(e.DB()).WithBlobstore(blob)
	staged, err := store.StageCapturedFiles(capture, []activities.CapturedFile{{
		PartID: "1", Filename: "invoice.html", ContentType: "text/html",
		Body: []byte(`<html><script>document.title="xss"</script></html>`),
	}})
	if err != nil {
		t.Fatalf("staging the markup file: %v", err)
	}
	if err := database.WithWorkspaceTx(capture, e.Pool, func(tx pgx.Tx) error {
		return store.RecordCapturedFiles(capture, tx, ids.From[ids.ActivityKind](email), activities.CapturedFileSource{
			System: "imap", MessageID: "invoice-" + email.String(),
			CapturedBy: "connector:imap", Category: "email_attachment",
		}, staged)
	}); err != nil {
		t.Fatalf("recording the markup file: %v", err)
	}
	att := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM attachment WHERE entity_id = $1`, email))

	dl := httptest.NewRecorder()
	h.DownloadAttachment(dl, httptest.NewRequest(http.MethodGet, "/v1/attachments/"+att.String(), nil).WithContext(ctx), crmcontracts.Id(att))
	if dl.Code != http.StatusOK {
		t.Fatalf("download: status %d", dl.Code)
	}
	if got := dl.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment;") {
		t.Fatalf("Content-Disposition is %q; an attachment must be saved, not rendered — "+
			"a sniffed text/html file served inline is stored XSS", got)
	}
}

// An uploaded filename is made safe where it enters the row, not where it is
// shown.
//
// The name is typed by whoever produced the file, and it is read back in a log
// line, a CSV export, a list, and — since a channel reply can carry files — a
// park reason a reader reads to find out which file to fix. A name carrying a
// line break rewrites whichever record quotes it, and one carrying a
// bidirectional override renders as an extension it does not have. The capture
// path has always run sender-supplied names through this; an upload is the same
// untrusted string arriving by a different door.
func TestAnUploadedFilenameIsSanitizedBeforeItIsStored(t *testing.T) {
	e := Setup(t)
	h := activities.NewHandlers(e.DB()).WithUploadLimit(uploadCeiling).WithBlobstore(blobstore.NewMemory())
	ctx := e.Admin()
	contact := e.SeedContact(t, "Untrusted Name", &e.Rep1)

	// A newline, a path separator and a right-to-left override: the three classes
	// SafeFilename exists for, in one name.
	body, ctype := multipartAttachment(t, "contact", contact.String(),
		"../etc/quo\u202ete\nreason=sent.pdf", []byte("PDF-BYTES"))
	req := httptest.NewRequest(http.MethodPost, "/v1/attachments", body).WithContext(ctx)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.UploadAttachment(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: status %d, body %s", rec.Code, rec.Body.String())
	}
	var att crmcontracts.Attachment
	if err := json.Unmarshal(rec.Body.Bytes(), &att); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if strings.ContainsAny(att.Filename, "/\\\n") {
		t.Errorf("the stored filename is %q; a separator or a line break in it rewrites every record that quotes it", att.Filename)
	}
	if strings.ContainsRune(att.Filename, '\u202e') {
		t.Errorf("the stored filename %q keeps a bidirectional override, so it renders as an extension it does not have", att.Filename)
	}
}
