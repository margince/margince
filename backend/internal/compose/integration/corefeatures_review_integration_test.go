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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func TestHiddenContactsStillReceiveTheirAccessAndErasureRights(t *testing.T) {
	var disabled []string
	e := apptest.SetupAppWithOriginOptions(t, func(string) []compose.Option { return []compose.Option{compose.WithDisabledCoreFeatures(disabled)} })
	e.BootstrapWorkspace(t)
	contact := createdRecord(t, e, "/v1/contacts", AnyMap{"full_name": "Rights subject", "source": "manual"})
	due := clocktest.Now(t).Add(24 * time.Hour).UTC().Format(time.RFC3339)
	access := createdRecord(t, e, "/v1/data-subject-requests", AnyMap{"kind": "access", "subject_ref": contact, "due_at": due})
	erasure := createdRecord(t, e, "/v1/data-subject-requests", AnyMap{"kind": "erasure", "subject_ref": contact, "due_at": due})
	disabled = []string{"contacts"}
	e.Close()
	e = e.Reboot(t)
	mustCall(t, e, http.MethodGet, "/v1/contacts/"+contact, nil, http.StatusNotFound, nil)
	mustCall(t, e, http.MethodPatch, "/v1/data-subject-requests/"+access, AnyMap{"status": "in_progress", "identity_check": "id_document"}, http.StatusOK, nil)
	var pkg privacy.SARPackage
	mustCall(t, e, http.MethodGet, "/v1/data-subject-requests/"+access+"/package", nil, http.StatusOK, &pkg)
	subject, err := json.Marshal(pkg.Subject)
	if err != nil || !strings.Contains(string(subject), "Rights subject") {
		t.Fatalf("access package lost the subject: %s, %v", subject, err)
	}
	mustCall(t, e, http.MethodPatch, "/v1/data-subject-requests/"+erasure, AnyMap{"status": "fulfilled", "identity_check": "id_document"}, http.StatusOK, nil)
	var archived bool
	if err := e.Owner.QueryRow(t.Context(), `SELECT archived_at IS NOT NULL FROM contact WHERE id=@id`, pgx.StrictNamedArgs{"id": contact}).Scan(&archived); err != nil || !archived {
		t.Fatalf("erasure did not run: %v, %v", archived, err)
	}
}

func TestPublicCardsHideDisabledRecordsAndDependentCompanyNames(t *testing.T) {
	var disabled []string
	e := apptest.SetupAppWithOriginOptions(t, func(string) []compose.Option { return []compose.Option{compose.WithDisabledCoreFeatures(disabled)} })
	e.BootstrapWorkspace(t)
	company := createdRecord(t, e, "/v1/companies", AnyMap{"display_name": "Hidden employer", "source": "manual"})
	contact := createdRecord(t, e, "/v1/contacts", AnyMap{"full_name": "Visible contact", "title": "Director", "company_id": company, "source": "manual"})
	disabled = []string{"companies"}
	e.Close()
	e = e.Reboot(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/public/record-previews/contacts/"+contact, nil)
	response := httptest.NewRecorder()
	e.TS.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Visible contact") || strings.Contains(response.Body.String(), "Hidden employer") {
		t.Fatalf("public card: %d %s", response.Code, response.Body)
	}
	disabled = []string{"contacts"}
	e.Close()
	e = e.Reboot(t)
	response = httptest.NewRecorder()
	e.TS.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "Visible contact") {
		t.Fatalf("disabled public card: %d %s", response.Code, response.Body)
	}
}

func TestAQueuedMessageRechecksFeatureAvailabilityWhenItFires(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	company := p.seedCompany(t, "Queued company")
	var scheduled struct {
		ID string `json:"id"`
	}
	status := p.Call(t, http.MethodPost, "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Kept scheduled message", "body": "Scheduled before the feature was hidden.", "to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": clocktest.Now(t).Add(2 * time.Hour).UTC().Format(time.RFC3339), "scheduled_tz": "Europe/Berlin",
		"also_links": []AnyMap{{"entity_type": "company", "entity_id": company.String()}},
	}, nil, &scheduled)
	if status != http.StatusCreated {
		t.Fatalf("schedule: %d", status)
	}
	id := uuidOf(t, scheduled.ID)
	before := p.countDeliveries(t)
	p.makeDue(t, id)
	if err := compose.DriveScheduledSendForTest(context.Background(), p.Pool, p.workspaceID(t), id, compose.SendOrigin{PublicBaseURL: preflightBaseURL}, "companies"); err != nil {
		t.Fatal(err)
	}
	if p.sent(t, id) || p.countDeliveries(t) != before {
		t.Fatal("a queued send used a disabled company")
	}
}
