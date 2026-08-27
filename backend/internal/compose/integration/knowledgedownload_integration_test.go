// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The download route, which is what a CITATION POINTS AT. An answer names the
// document a sentence rests on and quotes it; without a way to open the file
// the reader cannot see the quote in place, and a citation nobody can follow is
// a citation in name only.
//
// What only exists at this layer: the bytes come back BYTE-IDENTICAL to what
// was uploaded, the filename survives to the Content-Disposition so a browser
// saves it under its own name, and a reader who is not granted the document
// object is refused rather than served.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The bytes a reader gets back are the bytes that were uploaded. Anything else
// — a re-encode, a truncation at a buffer boundary, the extracted text instead
// of the file — and the reader is checking a citation against something other
// than the document that was cited.
func TestADocumentComesBackByteIdenticalToWhatWasUploaded(t *testing.T) {
	e := Setup(t)
	h := newKnowledgeHTTP(e)
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	made := httpCorpus(ctx, t, h)

	// Deliberately not plain ASCII: an umlaut and a trailing newline are what a
	// re-encoding or a trimming round trip would quietly change.
	const original = "# Betrieb\n\nDie Löschfrist beträgt 400 Tage.\n"

	body, ctype := multipartDocument(t, "handbuch.md", "text/markdown", []byte(original))
	upRec := httptest.NewRecorder()
	upReq := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", body).WithContext(ctx)
	upReq.Header.Set("Content-Type", ctype)
	h.handlers.UploadCorpusDocument(upRec, upReq, made.Id)
	if upRec.Code != http.StatusAccepted {
		t.Fatalf("upload: status %d, body %s", upRec.Code, upRec.Body.String())
	}
	var doc crmcontracts.KnowledgeDocument
	if err := json.Unmarshal(upRec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}

	downRec := httptest.NewRecorder()
	h.handlers.DownloadCorpusDocument(downRec,
		httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/x", nil).WithContext(ctx), doc.Id)
	if downRec.Code != http.StatusOK {
		t.Fatalf("download: status %d, body %s", downRec.Code, downRec.Body.String())
	}
	if got := downRec.Body.String(); got != original {
		t.Fatalf("download returned %q, want the uploaded bytes %q", got, original)
	}

	// The filename has to survive, or the browser saves the file under the
	// opaque id and the reader has no idea which document they opened.
	if cd := downRec.Header().Get("Content-Disposition"); !strings.Contains(cd, "handbuch.md") {
		t.Fatalf("Content-Disposition = %q, naming nothing the reader uploaded", cd)
	}
}

// A reader granted nothing on knowledge_document does not get the file. The
// download is a READ of the document, so it carries the same gate every other
// read does rather than being open because it streams bytes.
func TestDownloadingWithoutTheDocumentGrantIsRefused(t *testing.T) {
	e := Setup(t)
	h := newKnowledgeHTTP(e)
	admin := e.As(e.Rep1, nil, corpusAdminPerms)
	made := httpCorpus(admin, t, h)

	body, ctype := multipartDocument(t, "operating.md", "text/markdown", []byte(onePassage))
	upRec := httptest.NewRecorder()
	upReq := httptest.NewRequest(http.MethodPost, "/v1/knowledge/corpora/x/documents", body).WithContext(admin)
	upReq.Header.Set("Content-Type", ctype)
	h.handlers.UploadCorpusDocument(upRec, upReq, made.Id)
	var doc crmcontracts.KnowledgeDocument
	if err := json.Unmarshal(upRec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}

	// A REP, who holds knowledge_document:read, must be served — the person who
	// received a cited answer is exactly the person who needs to open what it
	// cited, and a download only admins can use makes every citation
	// uncheckable for everyone else.
	rep := e.As(e.Rep1, nil, corpusRepPerms)
	repRec := httptest.NewRecorder()
	h.handlers.DownloadCorpusDocument(repRec,
		httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/x", nil).WithContext(rep), doc.Id)
	if repRec.Code != http.StatusOK {
		t.Fatalf("rep download: status %d, want 200; body %s", repRec.Code, repRec.Body.String())
	}

	ungranted := e.As(e.Rep2, nil, corpusNoGrantPerms)
	rec := httptest.NewRecorder()
	h.handlers.DownloadCorpusDocument(rec,
		httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/x", nil).WithContext(ungranted), doc.Id)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("download without the grant: status %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), onePassage) {
		t.Fatal("a refused download leaked the document's contents into its error body")
	}
}

// An id that names no document is 404, and the body says nothing about whether
// some other organization holds one — existence stays hidden.
func TestDownloadingAnUnknownDocumentIs404(t *testing.T) {
	e := Setup(t)
	h := newKnowledgeHTTP(e)
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	rec := httptest.NewRecorder()
	h.handlers.DownloadCorpusDocument(rec,
		httptest.NewRequest(http.MethodGet, "/v1/knowledge/documents/x", nil).WithContext(ctx), crmcontracts.Id(ids.NewV7()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("download of an unknown id: status %d, want 404", rec.Code)
	}
}

// corpusNoGrantPerms holds a seat with no knowledge grant at all — the shape of
// a role an operator never added the objects to.
var corpusNoGrantPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects:  map[string]principal.ObjectGrant{},
	RowScope: principal.RowScopeAll,
}
