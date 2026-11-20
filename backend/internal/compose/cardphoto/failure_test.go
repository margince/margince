// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package cardphoto

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// failingCardLane is a lane that carries the photo and ends without an answer.
type failingCardLane struct{ err error }

func (failingCardLane) AttachmentMIMEs() []string { return []string{"image/png"} }

func (l failingCardLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, l.err
}

// importCardPhotoThrough posts a photo to the contacts route, served by the
// production reader over lane, as a member who may import.
func importCardPhotoThrough(t *testing.T, lane ai.ImageLane) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "cards.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(cardPNG(t)); err != nil {
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
			Objects: map[string]principal.ObjectGrant{"contact": {Create: true, Update: true}},
		},
	})
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/contacts/card-photos", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	contacts.Handlers{}.WithCardPhotoReader(Reader(lane)).ImportCardPhoto(rec, req)
	return rec
}

// A user waiting on a card photo is told why the reading did not happen and
// what to do: the budget stop, a provider the router blocked and a lane that
// ended without an answer each answer 503 with a code the screen has copy for,
// never 500 "internal".
func TestACardPhotoTheModelDidNotReadSaysWhy(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"budget used up": {
			err:  &ai.BudgetDeferralError{Task: ai.TaskDocumentExtract, NextAttemptAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
			code: "ai_allowance_used_up",
		},
		"provider down": {
			err: &ai.ProviderDownError{Provider: "p", Health: model.HealthDown,
				RetryAfter: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)},
			code: "provider_unavailable",
		},
		"every tier failed": {err: ai.ErrAllTiersFailed, code: "assistant_unavailable"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := importCardPhotoThrough(t, failingCardLane{err: tc.err})
			var problem struct{ Code string }
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decoding %s: %v", rec.Body, err)
			}
			if rec.Code != http.StatusServiceUnavailable || problem.Code != tc.code {
				t.Errorf("status = %d, code = %q, want 503 %q: %s", rec.Code, problem.Code, tc.code, rec.Body)
			}
		})
	}
}
