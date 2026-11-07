// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// repWith is a human seat holding grant on contact; the refusals below precede
// every database read, so a zero-value Store answers them.
func repWith(grant principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"contact": grant}},
	})
}

func TestAPreviewNeedsTheGrantTheSendNeeds(t *testing.T) {
	_, err := (&Store{}).PreviewControllerMail(repWith(principal.ObjectGrant{Read: true}), ids.ContactID{}, LinkRecordConfirmation, "")
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader without contact:update previewed: %v, want permission denied", err)
	}
}

func TestAPreviewRefusesAKindItDoesNotSend(t *testing.T) {
	var refusal *ValidationError
	_, err := (&Store{}).PreviewControllerMail(repWith(principal.ObjectGrant{Update: true}), ids.ContactID{}, LinkConsentConfirmation, "")
	if !errors.As(err, &refusal) || refusal.Field != fieldKind {
		t.Errorf("a consent link preview answered %v, want a refusal of kind", err)
	}
}

func TestAPreviewRefusesALanguageAndNamesTheOnesOnOffer(t *testing.T) {
	var refusal *ValidationError
	_, err := (&Store{}).PreviewControllerMail(repWith(principal.ObjectGrant{Update: true}), ids.ContactID{}, LinkPrivacyNotice, "fr")
	var offered []string
	for _, language := range mailcopy.Languages() {
		offered = append(offered, string(language))
	}
	if !errors.As(err, &refusal) || refusal.Field != "language" || !strings.Contains(refusal.Reason, strings.Join(offered, ", ")) {
		t.Errorf("a preview in fr answered %v, want a refusal of language naming %v", err, offered)
	}
}

func TestAConfirmRequestRefusesALanguageItCannotWriteIn(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/contacts/x/consent/confirm-request", strings.NewReader(`{"language":"fr"}`)).
		WithContext(repWith(principal.ObjectGrant{Update: true}))
	Handlers{store: &Store{}}.RequestDetailsConfirmation(rec, req, crmcontracts.Id{})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "language") {
		t.Fatalf("status = %d (body: %s), want a 422 naming the language", rec.Code, rec.Body.String())
	}
}

func TestAnEmptySendBodyPicksNoLanguageHoweverItArrives(t *testing.T) {
	for _, length := range []int64{0, -1} {
		req := httptest.NewRequest(http.MethodPost, "/contacts/x/consent/confirm-request", strings.NewReader(""))
		req.ContentLength = length
		rec := httptest.NewRecorder()
		if language, ok := sendLanguageOf(rec, req); !ok || language != "" {
			t.Errorf("content length %d: language %q ok %v (status %d), want no language and no refusal",
				length, language, ok, rec.Code)
		}
	}
}
