// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func confirmRequestWith(e *channelConsentEnv, h Handlers, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/contacts/x/consent/confirm-request", strings.NewReader(body)).
		WithContext(e.ctx)
	h.RequestDetailsConfirmation(rec, req, crmcontracts.Id(e.contact.UUID))
	return rec
}

func TestAConfirmRequestBodyPicksTheLanguageOfTheMail(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := &recordingStager{}

	if rec := confirmRequestWith(e, withLane(e, stager, &recordingVault{}), `{"language":"vi"}`); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	if stager.seen.Rendered.Language != "vi" {
		t.Errorf("the mail was rendered in %q, want the picked vi", stager.seen.Rendered.Language)
	}
}

func TestThePreviewAnswersTheMailWithEveryLanguageOnOffer(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := &recordingStager{}
	h := withLane(e, stager, &recordingVault{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/contacts/x/consent/mail-preview", nil).WithContext(e.ctx)
	german := crmcontracts.MailLanguage("de")
	h.PreviewControllerMail(rec, req, crmcontracts.Id(e.contact.UUID), crmcontracts.PreviewControllerMailParams{
		Kind: crmcontracts.PreviewControllerMailParamsKind(LinkPrivacyNotice), Language: &german,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got crmcontracts.ControllerMailPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Language != "de" || got.Kind != crmcontracts.ControllerMailPreviewKind(LinkPrivacyNotice) {
		t.Errorf("preview %s in %s, want the privacy notice in de", got.Kind, got.Language)
	}
	if got.ToAddress != "subject-"+e.contact.String()+"@example.test" {
		t.Errorf("to_address %q, want the subject's own address", got.ToAddress)
	}
	var carried []crmcontracts.MailLanguage
	for _, language := range mailcopy.Languages() {
		carried = append(carried, crmcontracts.MailLanguage(language))
	}
	if !slices.Equal(got.Languages, carried) {
		t.Errorf("languages %v, want every language the catalog carries %v", got.Languages, carried)
	}
	if !strings.Contains(got.Text, mailcopy.For("de").PreviewLinkMarker) {
		t.Errorf("the preview text carries no link marker:\n%s", got.Text)
	}
	if stager.calls != 0 {
		t.Error("opening a preview staged a mail")
	}
}

func TestThePageALinkOpensAsksInTheLanguageItsMailWasWrittenIn(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	vault := &recordingVault{}
	h := withLane(e, &recordingStager{}, vault)
	if rec := confirmRequestWith(e, h, `{"language":"de"}`); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	_, token, _ := strings.Cut(vault.sealed, "#/confirm/")

	rec := httptest.NewRecorder()
	h.GetConfirmDetails(rec, httptest.NewRequest(http.MethodGet, "/public/confirm/x", nil).WithContext(e.ctx), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var page crmcontracts.RecordConfirmationPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Language == nil || *page.Language != "de" {
		t.Errorf("the page names language %v, want de, the language its mail was written in", page.Language)
	}
}
