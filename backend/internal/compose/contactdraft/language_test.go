// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The language the rep chose writes the draft and is the one it reports. With
// none chosen, the draft reports the language the server resolved.
func TestAContactDraftIsWrittenInTheChosenLanguage(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body     string
		want     crmcontracts.CompanyEmailDraftLanguage
		greeting string
	}{
		"Vietnamese chosen":   {`{"language":"vi"}`, crmcontracts.CompanyEmailDraftLanguageVi, "Chào Ada,"},
		"German chosen":       {`{"language":"de"}`, crmcontracts.CompanyEmailDraftLanguageDe, "Guten Tag Ada"},
		"nothing chosen":      {`{}`, crmcontracts.CompanyEmailDraftLanguageEn, "Hi Ada,"},
		"an empty request":    {``, crmcontracts.CompanyEmailDraftLanguageEn, "Hi Ada,"},
		"intent and language": {`{"intent":"Ask about the pilot in May","language":"vi"}`, crmcontracts.CompanyEmailDraftLanguageVi, "Chào Ada,"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := draftContactAs(t, tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("draft answered %d: %s", rec.Code, rec.Body)
			}
			var draft crmcontracts.CompanyEmailDraft
			if err := json.Unmarshal(rec.Body.Bytes(), &draft); err != nil {
				t.Fatalf("decoding the draft: %v", err)
			}
			if draft.Language == nil || *draft.Language != tc.want {
				t.Fatalf("draft reports %v, want %s", draft.Language, tc.want)
			}
			if !strings.HasPrefix(draft.Body, tc.greeting) {
				t.Errorf("draft opens %q, want %q", draft.Body, tc.greeting)
			}
		})
	}
}

func TestAContactDraftInALanguageTheProductDoesNotWriteIsRefused(t *testing.T) {
	t.Parallel()
	rec := draftContactAs(t, `{"language":"fr"}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"language"`) {
		t.Fatalf("an unknown language answered %d: %s, want 422 naming language", rec.Code, rec.Body)
	}
}

func draftContactAs(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	pathID, path := contactAt("ada@customer.test")
	seat := ids.NewV7()
	human := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
	})
	req := httptest.NewRequestWithContext(human, http.MethodPost, "/contacts/x/draft-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	contactdraft.NewHandlers(contactdraft.NewService(pathContact{path}, nil)).
		DraftContactEmail(rec, req, openapi_types.UUID(pathID.UUID))
	return rec
}
