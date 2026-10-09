// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// A file outside the accepted kinds is refused before anything is stored: no
// row, and no object left in the bucket for a later sweep to find.
//
// The PDF beside it is the control, because a rule that refused every upload
// would satisfy the SVG case on its own.
func TestAnUploadOfAnUnacceptedKindStoresNothing(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	h := activities.NewHandlers(e.DB()).WithUploadLimit(uploadCeiling).WithBlobstore(blob)
	ctx := e.Admin()
	contact := e.SeedContact(t, "Typed Upload", &e.Rep1)

	upload := func(filename string, data []byte) *httptest.ResponseRecorder {
		body, ctype := multipartAttachment(t, "contact", contact.String(), filename, data)
		req := httptest.NewRequest(http.MethodPost, "/v1/attachments", body).WithContext(ctx)
		req.Header.Set("Content-Type", ctype)
		rec := httptest.NewRecorder()
		h.UploadAttachment(rec, req)
		return rec
	}

	refused := upload("logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	if refused.Code != http.StatusUnprocessableEntity {
		t.Fatalf("uploading an SVG file: status %d, want 422; body %s", refused.Code, refused.Body.String())
	}
	var problem struct {
		Details struct {
			Errors []struct{ Field, Code string } `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(refused.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decoding the refusal: %v", err)
	}
	if faults := problem.Details.Errors; len(faults) != 1 || faults[0].Field != "file" || faults[0].Code != "unsupported_file_type" {
		t.Errorf("the refusal names %+v, want one file/unsupported_file_type field", faults)
	}
	if got := refused.Body.String(); !strings.Contains(got, "logo.svg") || !strings.Contains(got, "PDF") {
		t.Errorf("the refusal does not name the file and what is accepted: %s", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM attachment WHERE entity_id = $1`, contact); n != 0 {
		t.Errorf("a refused upload left %d attachment rows", n)
	}
	if n, err := blob.DeletePrefix(context.Background(), e.WS.String()+"/"); err != nil || n != 0 {
		t.Errorf("a refused upload left %d stored objects (err %v)", n, err)
	}

	accepted := upload("quote.pdf", []byte("%PDF-1.4"))
	if accepted.Code != http.StatusCreated {
		t.Fatalf("a PDF was refused too: status %d, body %s", accepted.Code, accepted.Body.String())
	}
	var att crmcontracts.Attachment
	if err := json.Unmarshal(accepted.Body.Bytes(), &att); err != nil {
		t.Fatalf("decoding the stored PDF: %v", err)
	}
	if att.ContentType == nil || *att.ContentType != "application/pdf" {
		t.Errorf("a PDF sent as octet-stream is stored as %v, want application/pdf", att.ContentType)
	}
}
