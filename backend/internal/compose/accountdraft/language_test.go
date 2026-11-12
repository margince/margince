// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fixedAccount answers one assembled account, whatever is asked.
type fixedAccount struct{ view crmcontracts.Company360 }

func (a fixedAccount) AssembleScoped(context.Context, ids.CompanyID, company360.AssembleOptions) (crmcontracts.Company360, error) {
	return a.view, nil
}

// A language the rep chose, then the subject they wrote, outranks the account's
// German correspondence. The draft reports the language it is written in.
func TestAnAccountDraftIsWrittenInTheChosenLanguage(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		language string
		want     crmcontracts.CompanyEmailDraftLanguage
	}{
		"Vietnamese chosen over German mail": {`,"language":"vi"`, crmcontracts.CompanyEmailDraftLanguageVi},
		"English chosen over German mail":    {`,"language":"en"`, crmcontracts.CompanyEmailDraftLanguageEn},
		"nothing chosen follows the mail":    {``, crmcontracts.CompanyEmailDraftLanguageDe},
		"an English subject over German mail": {
			`,"subject":"Your proposal and our next steps"`, crmcontracts.CompanyEmailDraftLanguageEn,
		},
		"a chosen language over the subject": {
			`,"subject":"Your proposal and our next steps","language":"vi"`, crmcontracts.CompanyEmailDraftLanguageVi,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := draftAccountAs(t, tc.language)
			if rec.Code != http.StatusOK {
				t.Fatalf("draft answered %d: %s", rec.Code, rec.Body)
			}
			var draft crmcontracts.CompanyEmailDraft
			if err := json.Unmarshal(rec.Body.Bytes(), &draft); err != nil {
				t.Fatalf("decoding the draft: %v", err)
			}
			if draft.Language == nil || *draft.Language != tc.want {
				t.Errorf("draft reports %v, want %s", draft.Language, tc.want)
			}
		})
	}
}

func TestAnAccountDraftInALanguageTheProductDoesNotWriteIsRefused(t *testing.T) {
	t.Parallel()
	rec := draftAccountAs(t, `,"language":"fr"`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"language"`) {
		t.Fatalf("an unknown language answered %d: %s, want 422 naming language", rec.Code, rec.Body)
	}
}

// draftAccountAs drafts to the one contact of an account whose only
// correspondence is German.
func draftAccountAs(t *testing.T, language string) *httptest.ResponseRecorder {
	t.Helper()
	contact := ids.NewV7()
	view := viewWithActivities(t, activityWith(t, "Angebot",
		"Guten Tag, wir haben das Angebot geprüft und melden uns wegen der offenen Punkte bei Ihnen."))
	view.Contacts = &struct {
		Data []crmcontracts.Company360Contact `json:"data"`
		Page crmcontracts.PageInfo            `json:"page"`
	}{Data: []crmcontracts.Company360Contact{{ContactId: openapi_types.UUID(contact), FullName: "Priya Raman"}}}
	seat := ids.NewV7()
	human := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
	})
	body := `{"contact_id":"` + contact.String() + `"` + language + `}`
	req := httptest.NewRequestWithContext(human, http.MethodPost, "/companies/x/draft-email", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewHandlers(NewService(fixedAccount{view: view}, nil)).DraftCompanyEmail(rec, req, openapi_types.UUID(ids.NewV7()))
	return rec
}
