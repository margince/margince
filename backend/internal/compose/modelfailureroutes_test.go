// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/companydossier"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/leaddraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Answering a model route's error through modelfailure changes how a model lane
// that did not answer is told, and nothing else: a refusal that is not a model
// outcome — here, a caller with no principal — reaches the client exactly as
// httperr.Write answers it, never as the assistant being unavailable.
func TestAModelRouteAnswersARefusalThatIsNotAModelOutcomeAsBefore(t *testing.T) {
	refusal := auth.RequireHuman(context.Background())
	if refusal == nil {
		t.Fatal("a request with no principal was admitted — this test has nothing to refuse")
	}
	want := httptest.NewRecorder()
	httperr.Write(want, httptest.NewRequest(http.MethodGet, "/", nil), refusal)
	if want.Code == http.StatusServiceUnavailable {
		t.Fatalf("httperr answers the refusal %d, which cannot be told apart from a model outcome", want.Code)
	}

	id := crmcontracts.Id(uuid.New())
	contactBody := `{"contact_id":"` + uuid.NewString() + `"}`
	introBody := `{"contact_id":"` + uuid.NewString() + `","via_user_id":"` + uuid.NewString() + `"}`
	routes := map[string]struct {
		body  string
		serve func(http.ResponseWriter, *http.Request)
	}{
		"accountdraft DraftCompanyEmail": {contactBody, func(w http.ResponseWriter, r *http.Request) {
			accountdraft.NewHandlers(new(accountdraft.Service)).DraftCompanyEmail(w, r, id)
		}},
		"company360 DraftIntroRequest": {introBody, func(w http.ResponseWriter, r *http.Request) {
			company360.NewHandlers(new(company360.Service)).DraftIntroRequest(w, r, id)
		}},
		"companybrief GetCompanyBrief": {"", func(w http.ResponseWriter, r *http.Request) {
			companybrief.NewHandlers(new(companybrief.Service)).GetCompanyBrief(w, r, id, crmcontracts.GetCompanyBriefParams{})
		}},
		"companybrief AskAboutCompany": {`{}`, func(w http.ResponseWriter, r *http.Request) {
			companybrief.NewHandlers(new(companybrief.Service)).AskAboutCompany(w, r, id)
		}},
		"companydossier GetCompanyDossier": {"", func(w http.ResponseWriter, r *http.Request) {
			companydossier.NewHandlers(new(companydossier.Service), nil).GetCompanyDossier(w, r, id)
		}},
		"companydossier GetCompanyGrowthFit": {"", func(w http.ResponseWriter, r *http.Request) {
			companydossier.NewHandlers(nil, new(companydossier.GrowthFitService)).GetCompanyGrowthFit(w, r, id)
		}},
		"contactbrief GetContactBrief": {"", func(w http.ResponseWriter, r *http.Request) {
			contactbrief.NewHandlers(new(contactbrief.Service)).GetContactBrief(w, r, id)
		}},
		"contactdraft DraftContactEmail": {"", func(w http.ResponseWriter, r *http.Request) {
			contactdraft.NewHandlers(new(contactdraft.Service)).DraftContactEmail(w, r, id)
		}},
		"leaddraft DraftLeadEmail": {"", func(w http.ResponseWriter, r *http.Request) {
			leaddraft.NewHandlers(new(leaddraft.Service)).DraftLeadEmail(w, r, id)
		}},
	}
	for name, route := range routes {
		t.Run(name, func(t *testing.T) {
			got := httptest.NewRecorder()
			route.serve(got, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(route.body)))
			if got.Code != want.Code || got.Body.String() != want.Body.String() {
				t.Errorf("answered %d %s; httperr answers the same refusal %d %s",
					got.Code, got.Body.String(), want.Code, want.Body.String())
			}
		})
	}
}
