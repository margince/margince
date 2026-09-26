// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The analytics routes as a client meets them: a refusal carries its kind and
// its suggestion as fields, and a saved run echoes the question it was asked
// with, scope included.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func (e *forecastEnv) analyticsRoutes() analyticsQueryHandlers {
	return newAnalyticsQueryHandlers(InstallationDB(e.Pool), analyticsquery.DefaultFloor)
}

func analyticsRequest(ctx context.Context, method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
}

// assertRefusal decodes a 400 through the contract's own type and checks the
// refusal's parts against the sentence detail carries.
func assertRefusal(t *testing.T, rec *httptest.ResponseRecorder, want crmcontracts.AnalyticsRefusalDetailsKind) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("answered %d (%s), want the 400 a refusal is", rec.Code, rec.Body.String())
	}
	var body crmcontracts.AnalyticsRefusal
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the refusal is not the contract's shape: %v", err)
	}
	if body.Details == nil {
		t.Fatalf("the refusal carries no details, so a client must parse the sentence: %s", rec.Body.String())
	}
	if body.Details.Kind != want {
		t.Errorf("details.kind is %q, want %q", body.Details.Kind, want)
	}
	if body.Details.Suggest == "" || body.Details.Message == "" {
		t.Errorf("details %+v leaves the client nothing to show", *body.Details)
	}
	sentence := string(want) + ": " + body.Details.Message + " — " + body.Details.Suggest
	if body.Detail == nil || *body.Detail != sentence {
		t.Errorf("detail is %v, want the sentence agents read, %q", body.Detail, sentence)
	}
}

func TestAnInvalidQuestionIsRefusedWithItsKind(t *testing.T) {
	e := setupForecast(t)
	rec := httptest.NewRecorder()
	e.analyticsRoutes().RunAnalyticsQuery(rec, analyticsRequest(e.reportReaderCtx(), http.MethodPost,
		"/analytics/query", `{"entity":"open-deals-per-company","measures":[{"fn":"count"}],
			"filters":[{"field":"owner_id","op":"is_null","value":"x"}]}`))
	assertRefusal(t, rec, crmcontracts.AnalyticsRefusalDetailsKindInvalid)
}

func TestAnUnknownPopulationIsRefusedAsUnsupported(t *testing.T) {
	e := setupForecast(t)
	rec := httptest.NewRecorder()
	e.analyticsRoutes().RunAnalyticsQuery(rec, analyticsRequest(e.reportReaderCtx(), http.MethodPost,
		"/analytics/query", `{"entity":"no-such-population","measures":[{"fn":"count"}]}`))
	assertRefusal(t, rec, crmcontracts.AnalyticsRefusalDetailsKindUnsupported)
}

func TestAFilterHidingTooFewRecordsIsRefusedAsPrivacy(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Big", 20, &e.Rep1, &amount, nil)
	}
	for i := 0; i < 2; i++ {
		e.seedOpenDeal(t, "Small", 20, &e.Rep3, &amount, nil)
	}
	rec := httptest.NewRecorder()
	e.analyticsRoutes().RunAnalyticsQuery(rec, analyticsRequest(e.reportReaderCtx(), http.MethodPost,
		"/analytics/query", `{"entity":"open-deals-per-company","measures":[{"fn":"count"}],
			"filters":[{"field":"owner_id","op":"ne","value":"`+e.Rep3.String()+`"}]}`))
	assertRefusal(t, rec, crmcontracts.AnalyticsRefusalDetailsKindPrivacy)
}

// A saved run echoes the scope it was asked with, so a client re-asking it from
// the echo asks the same question rather than the reader's default.
func TestASavedRunEchoesTheScopeItWasAskedWith(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &amount, nil)
	}
	routes := e.analyticsRoutes()
	ctx := e.wideLensCtx(e.Rep3)

	saved := httptest.NewRecorder()
	routes.RunAnalyticsQuery(saved, analyticsRequest(ctx, http.MethodPost, "/analytics/query",
		`{"entity":"open-deals-per-company","scope_kind":"owner","scope_id":"`+e.Rep1.String()+`",
			"measures":[{"fn":"count"}],"save":true}`))
	var answer crmcontracts.AnalyticsAnswer
	if err := json.Unmarshal(saved.Body.Bytes(), &answer); err != nil || answer.RunId == nil {
		t.Fatalf("saving the scoped question answered %d %s", saved.Code, saved.Body.String())
	}

	read := httptest.NewRecorder()
	routes.GetReportRun(read, analyticsRequest(ctx, http.MethodGet, "/analytics/runs/x", ""), *answer.RunId)
	var run crmcontracts.ReportRun
	if err := json.Unmarshal(read.Body.Bytes(), &run); err != nil {
		t.Fatalf("reading the run answered %d %s", read.Code, read.Body.String())
	}
	if run.Query.ScopeKind == nil || *run.Query.ScopeKind != ScopeKindOwner {
		t.Errorf("the run echoes scope_kind %v, want %q", run.Query.ScopeKind, ScopeKindOwner)
	}
	if run.Query.ScopeId == nil || *run.Query.ScopeId != openapi_types.UUID(e.Rep1) {
		t.Errorf("the run echoes scope_id %v, want the owner it was asked about, %s", run.Query.ScopeId, e.Rep1)
	}

	// The cell route refuses through the same shape: an ungrouped run has no
	// group key for a cell to name.
	cell := httptest.NewRecorder()
	routes.ExplainReportRunCell(cell, analyticsRequest(ctx, http.MethodPost, "/analytics/runs/x/cells/explain",
		`{"group":["x"]}`), *answer.RunId)
	assertRefusal(t, cell, crmcontracts.AnalyticsRefusalDetailsKindInvalid)
}
