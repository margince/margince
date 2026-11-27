// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A sender who asks for it gets the identical file a record already holds,
// instead of a second copy of the same bytes. Without the ask every upload is
// stored, as before.

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// uploadTo posts one file to the record through the real handler, asking for
// an identical file instead of a copy when reuse is set.
func uploadTo(ctx context.Context, t *testing.T, h activities.Handlers, contact ids.UUID, filename string, data []byte, reuse bool) (int, crmcontracts.Attachment) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	fields := map[string]string{"entity_type": "contact", "entity_id": contact.String()}
	if reuse {
		fields["reuse_identical"] = "true"
	}
	for name, value := range fields {
		if err := form.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/attachments", &body).WithContext(ctx)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	h.UploadAttachment(rec, req)
	var att crmcontracts.Attachment
	if rec.Code < 300 {
		if err := json.Unmarshal(rec.Body.Bytes(), &att); err != nil {
			t.Fatalf("decode upload response: %v", err)
		}
	}
	return rec.Code, att
}

func TestAnIdenticalUploadAttachesTheFileTheRecordHolds(t *testing.T) {
	e := Setup(t)
	h := activities.NewHandlers(e.DB()).WithUploadLimit(uploadCeiling).WithBlobstore(blobstore.NewMemory())
	ctx := e.Admin()
	contact := e.SeedContact(t, "Reuse Attach", &e.Rep1)
	offer := []byte("%PDF-1.4 the offer")

	status, first := uploadTo(ctx, t, h, contact, "offer.pdf", offer, true)
	if status != http.StatusCreated {
		t.Fatalf("first upload: status %d, want 201", status)
	}
	status, again := uploadTo(ctx, t, h, contact, "offer copy.pdf", offer, true)
	if status != http.StatusOK || again.Id != first.Id {
		t.Errorf("identical upload: status %d, id %s; want 200 and the stored %s", status, again.Id, first.Id)
	}
	status, other := uploadTo(ctx, t, h, contact, "terms.pdf", []byte("%PDF-1.4 the terms"), true)
	if status != http.StatusCreated || other.Id == first.Id {
		t.Errorf("different bytes: status %d, id %s; want a new file", status, other.Id)
	}
	if status, _ := uploadTo(ctx, t, h, contact, "offer.pdf", offer, false); status != http.StatusCreated {
		t.Errorf("an upload that did not ask: status %d, want 201 and a copy", status)
	}

	if got := e.WsCount(t, `SELECT count(*) FROM attachment
		WHERE entity_id = $1 AND archived_at IS NULL`, contact); got != 3 {
		t.Errorf("%d files on the contact, want 3: one offer, the terms, and the copy asked for", got)
	}
}
