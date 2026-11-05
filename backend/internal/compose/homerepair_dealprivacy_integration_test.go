// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedSuggestionRepairRechecksAuthorityAtExecution(t *testing.T) {
	for _, change := range []string{"source_hidden", "grant_removed"} {
		t.Run(change, func(t *testing.T) { runSuggestionRepairAuthorityChange(t, change) })
	}
}

func runSuggestionRepairAuthorityChange(t *testing.T, change string) {
	e := setupScout(t)
	e.now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Example buyer", nil)
	source, buyer := e.buyingEmail(t, company, e.daysAgo(6))
	e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
	e.signal(t, "commitment_made", company, source, e.now, ids.Nil)
	e.pass(t)
	permissions := integration.AdminWithSignals
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects["data_task"] = principal.ObjectGrant{Read: true, Update: true}
	reader := e.As(e.Rep1, nil, permissions)
	e.GrantRole(t, e.Rep1, "admin")
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,data_task}','{"read":true,"update":true,"create":false,"delete":false}'::jsonb) WHERE key='admin'`)
	suggestion := e.onlySuggestion(reader, t, company)
	later := e.now.Add(dealScoutWindow + 24*time.Hour)
	h := dataTaskHandlers{pool: e.Pool, clock: func() time.Time { return later }}
	response := httptest.NewRecorder()
	h.PreviewHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/preview",
		bytes.NewBufferString(`{"source":"suggestion","limit":25}`)).WithContext(reader))
	if response.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", response.Code, response.Body.String())
	}
	var wire crmcontracts.HomeRepairReview
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Tasks) != 1 || wire.Counts.Supersede != 1 || wire.Counts.Withdraw != 0 || wire.Counts.Archive != 0 {
		t.Fatalf("review=%+v", wire)
	}
	engine, err := datatask.New(e.Pool, []datatask.Task{homeRepairDataTask(e.Pool, func() time.Time { return later })}, "test", func() time.Time { return later }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	system := principal.WithCorrelationID(principal.SystemActing(reader, "system:home-repair-test"), ids.NewV7())
	if err := engine.Seed(system); err != nil {
		t.Fatal(err)
	}
	activation, err := json.Marshal(crmcontracts.ActivateHomeRepairJSONBody{Version: 1, Review: wire})
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	h.ActivateHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/activate", bytes.NewReader(activation)).WithContext(reader))
	if response.Code != http.StatusNoContent {
		t.Fatalf("activate: %d %s", response.Code, response.Body.String())
	}
	detail := deals.NewHandlers(e.DB(), deals.Installation{})
	response = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/deal-suggestions/"+suggestion.ID.String(), nil).WithContext(reader)
	detail.GetDealSuggestion(response, request, crmcontracts.Id(suggestion.ID))
	if response.Code != http.StatusOK {
		t.Fatalf("inspect: %d %s", response.Code, response.Body.String())
	}
	if change == "source_hidden" {
		if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](source), activities.SetAudienceInput{
			Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}},
		}); err != nil {
			t.Fatal(err)
		}
		response = httptest.NewRecorder()
		detail.GetDealSuggestion(response, request, crmcontracts.Id(suggestion.ID))
		if response.Code != http.StatusNotFound {
			t.Fatalf("hidden source detail: %d %s", response.Code, response.Body.String())
		}
	} else {
		e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,deal,create}','false'::jsonb) WHERE key='admin'`)
	}
	if _, err := engine.Execute(system); err != nil {
		t.Fatal(err)
	}
	progress, err := engine.List(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0].Status != datatask.Done || progress[0].Changed != 0 {
		t.Fatalf("repair progress after authority changed=%+v", progress)
	}
	reason := "changed_since_review"
	if change == "grant_removed" {
		reason = "permission_no_longer_available"
	}
	if e.WsCount(t, "SELECT count(*) FROM system_log WHERE action='home_repair.skipped' AND detail->>'source'='suggestion' AND detail->>'reason'=$1", reason) != 1 {
		t.Fatal("repair did not report the skipped suggestion")
	}
	if state := e.WsScalar(t, "SELECT state FROM deal_suggestion WHERE id=$1", suggestion.ID); state != deals.SuggestionOpen {
		t.Fatalf("hidden-source suggestion became %s", state)
	}
}
