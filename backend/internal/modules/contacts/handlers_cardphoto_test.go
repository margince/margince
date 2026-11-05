// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// postCardPhoto sends a photo part of the given size to the route, as a member
// who may import, with a reader that counts its calls.
func postCardPhoto(t *testing.T, size int) (*httptest.ResponseRecorder, int) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "cards.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(make([]byte, size)); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	member := ids.NewV7()
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + member.String(), UserID: member,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{entityContact: {Create: true, Update: true}},
		},
	})
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/contacts/card-photos", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())

	reads := 0
	h := Handlers{}.WithCardPhotoReader(func(context.Context, []byte) ([]VCardEntry, error) {
		reads++
		return nil, &CardPhotoRefusal{Code: "no_cards", Message: "none"}
	})
	rec := httptest.NewRecorder()
	h.ImportCardPhoto(rec, req)
	return rec, reads
}

func TestACardPhotoOverTheImageBoundIsRefusedBeforeTheReading(t *testing.T) {
	rec, reads := postCardPhoto(t, imagenorm.MaxReadImageBytes+1)
	if rec.Code != http.StatusRequestEntityTooLarge || reads != 0 {
		t.Fatalf("status = %d, reads = %d, want 413 and no reading: %s", rec.Code, reads, rec.Body)
	}
}

func TestACardPhotoBodyOverTheCeilingIsRefusedWhileParsing(t *testing.T) {
	rec, reads := postCardPhoto(t, CardPhotoUploadBytes+1)
	if rec.Code != http.StatusRequestEntityTooLarge || reads != 0 {
		t.Fatalf("status = %d, reads = %d, want 413 and no reading: %s", rec.Code, reads, rec.Body)
	}
}

func TestACardPhotoAtTheImageBoundReachesTheReading(t *testing.T) {
	rec, reads := postCardPhoto(t, imagenorm.MaxReadImageBytes)
	if reads != 1 || rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, reads = %d, want the reader's own 422 after one reading", rec.Code, reads)
	}
}
