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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *forecastEnv) analyticsRoutes() analyticsQueryHandlers {
	return newAnalyticsQueryHandlers(InstallationDB(e.Pool), analyticsquery.DefaultFloor,
		newAttentionNames(InstallationDB(e.Pool)))
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

// leadReaderCtx may read leads across the workspace, which is what labelling
// them and measuring them both ask for.
func (e *forecastEnv) leadReaderCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:lead-reader", UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"lead": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func explainBody(t *testing.T, rec *httptest.ResponseRecorder) crmcontracts.AnalyticsExplanation {
	t.Helper()
	var out crmcontracts.AnalyticsExplanation
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("explaining answered %d %s", rec.Code, rec.Body.String())
	}
	return out
}

// An explanation names each record through the drawer's own seam. An archived
// lead is counted by leads-by-status and has no name the seam will give, so it
// keeps its id and carries no label.
func TestAnExplanationNamesTheRecordsTheReaderCanName(t *testing.T) {
	e := setupForecast(t)
	named := map[string]string{}
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("Live Lead %d", i)
		named[e.seedID(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
			VALUES ($1, $2, 'new', 'inbound', 'human:x', $3)`, name, e.Rep1).String()] = name
	}
	archived := e.seedID(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id, archived_at)
		VALUES ($1, 'Archived Lead', 'disqualified', 'inbound', 'human:x', $2, now())`, e.Rep1).String()

	rec := httptest.NewRecorder()
	e.analyticsRoutes().ExplainAnalyticsCell(rec, analyticsRequest(e.leadReaderCtx(), http.MethodPost,
		"/analytics/explain", `{"query":{"entity":"leads-by-status","measures":[{"fn":"count"}]}}`))
	out := explainBody(t, rec)

	if out.Columns[len(out.Columns)-1] != derivationLabelColumn {
		t.Errorf("columns %v do not announce the label", out.Columns)
	}
	seen := 0
	for _, row := range out.Rows {
		id, _ := row["id"].(string)
		switch {
		case id == archived:
			seen++
			if label, has := row[derivationLabelColumn]; has {
				t.Errorf("the archived lead carries label %v; the seam names no archived record", label)
			}
		case named[id] != "":
			seen++
			if row[derivationLabelColumn] != named[id] {
				t.Errorf("lead %s is labelled %v, want %q", id, row[derivationLabelColumn], named[id])
			}
		}
	}
	if seen != len(named)+1 {
		t.Fatalf("the explanation carried %d of the %d seeded leads", seen, len(named)+1)
	}
}

// A withheld cell opens to nothing, and says so with an empty list, not null.
func TestAWithheldCellExplainsToAnEmptyListOverTheWire(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Big", 20, &e.Rep1, &amount, nil)
	}
	for i := 0; i < 2; i++ {
		e.seedOpenDeal(t, "Small", 20, &e.Rep3, &amount, nil)
	}
	rec := httptest.NewRecorder()
	e.analyticsRoutes().ExplainAnalyticsCell(rec, analyticsRequest(e.reportReaderCtx(), http.MethodPost,
		"/analytics/explain", `{"query":{"entity":"open-deals-per-company","group_by":["owner_id"],
			"measures":[{"fn":"count"}]},"group":["`+e.Rep3.String()+`"]}`))
	if !strings.Contains(rec.Body.String(), `"rows":[]`) {
		t.Errorf("a withheld cell answered %s, want an empty rows list", rec.Body.String())
	}
	if out := explainBody(t, rec); !out.Withheld || len(out.Rows) != 0 {
		t.Errorf("a two-deal cell answered withheld=%v with %d rows", out.Withheld, len(out.Rows))
	}
}

// A saved run's cell is named through the same seam as a question's.
func TestACitedCellNamesItsRecords(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Cited Deal", 20, &e.Rep1, &amount, nil)
	}
	routes := e.analyticsRoutes()
	ctx := e.wideLensCtx(e.Rep3)
	saved := httptest.NewRecorder()
	routes.RunAnalyticsQuery(saved, analyticsRequest(ctx, http.MethodPost, "/analytics/query",
		`{"entity":"open-deals-per-company","scope_kind":"workspace","measures":[{"fn":"count"}],"save":true}`))
	var answer crmcontracts.AnalyticsAnswer
	if err := json.Unmarshal(saved.Body.Bytes(), &answer); err != nil || answer.RunId == nil {
		t.Fatalf("saving the question answered %d %s", saved.Code, saved.Body.String())
	}

	cell := httptest.NewRecorder()
	routes.ExplainReportRunCell(cell, analyticsRequest(ctx, http.MethodPost, "/analytics/runs/x/cells/explain", `{}`),
		*answer.RunId)
	out := explainBody(t, cell)
	if len(out.Rows) != 6 {
		t.Fatalf("the cell opened to %d records, want the 6 seeded", len(out.Rows))
	}
	for _, row := range out.Rows {
		if row[derivationLabelColumn] != "Cited Deal" {
			t.Errorf("record %v is labelled %v, want its name", row["id"], row[derivationLabelColumn])
		}
	}
}

// companyReaderCtx is a real seat that may read deals and the companies they
// belong to, and measure the whole workspace.
func (e *forecastEnv) companyReaderCtx(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:company-reader", UserID: user,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal": {Read: true}, "company": {Read: true}, "forecast": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// seedCompanyDeals plants a company and that many open deals at it.
func (e *forecastEnv) seedCompanyDeals(t *testing.T, name string, deals int) ids.UUID {
	t.Helper()
	company := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, $2, 'manual', 'human:x')`, name)
	for i := 0; i < deals; i++ {
		e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, owner_id, amount_minor,
		                               currency, expected_close_date, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, 100000, 'EUR', (now() + interval '30 days')::date, 'manual', 'human:x')`,
			name+" deal", e.pipeline, e.stages[20], company, e.Rep1)
	}
	return company
}

// An answer grouped by company names each served company, and names nothing a
// withheld row stood for: that row carries no id to name.
func TestAnAnswerNamesTheCompaniesItGroupsBy(t *testing.T) {
	e := setupForecast(t)
	names := map[string]string{}
	for _, c := range []struct {
		name  string
		deals int
	}{{"Contoso", 6}, {"Fabrikam", 7}, {"Tailspin", 1}} {
		names[e.seedCompanyDeals(t, c.name, c.deals).String()] = c.name
	}
	rec := httptest.NewRecorder()
	e.analyticsRoutes().RunAnalyticsQuery(rec, analyticsRequest(e.companyReaderCtx(e.Rep3), http.MethodPost,
		"/analytics/query", `{"entity":"open-deals-per-company","group_by":["company_id"],"measures":[{"fn":"count"}]}`))
	var answer crmcontracts.AnalyticsAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("asking answered %d %s", rec.Code, rec.Body.String())
	}
	if !answer.Withheld || answer.Labels == nil {
		t.Fatalf("want a withheld one-deal group beside named ones, got withheld=%v labels=%v",
			answer.Withheld, answer.Labels)
	}
	labels := (*answer.Labels)["company_id"]
	served := 0
	for _, row := range answer.Rows {
		id, _ := row["company_id"].(string)
		if row[withheldColumn] == true {
			if id != "" {
				t.Errorf("a withheld row still carries company %s", id)
			}
			continue
		}
		served++
		if labels[id] != names[id] {
			t.Errorf("company %s is named %q, want %q", id, labels[id], names[id])
		}
	}
	if served == 0 || len(labels) != served {
		t.Errorf("%d groups served and %d named — %v", served, len(labels), labels)
	}
}

// A saved run opened by a reader who may not read companies answers and opens
// its cells, naming the deals and none of the companies.
func TestANarrowerReaderOfARunIsNotHandedTheCompanyNames(t *testing.T) {
	e := setupForecast(t)
	company := e.seedCompanyDeals(t, "Contoso", 6).String()
	routes := e.analyticsRoutes()
	saved := httptest.NewRecorder()
	routes.RunAnalyticsQuery(saved, analyticsRequest(e.companyReaderCtx(e.Rep3), http.MethodPost, "/analytics/query",
		`{"entity":"open-deals-per-company","scope_kind":"workspace","group_by":["company_id"],
			"measures":[{"fn":"count"}],"save":true}`))
	var asked crmcontracts.AnalyticsAnswer
	if err := json.Unmarshal(saved.Body.Bytes(), &asked); err != nil || asked.RunId == nil || asked.Labels == nil {
		t.Fatalf("the saving reader's answer was %d %s", saved.Code, saved.Body.String())
	}

	narrow := e.wideLensCtx(e.Rep1)
	read := httptest.NewRecorder()
	routes.GetReportRun(read, analyticsRequest(narrow, http.MethodGet, "/analytics/runs/x", ""), *asked.RunId)
	var run crmcontracts.ReportRun
	if err := json.Unmarshal(read.Body.Bytes(), &run); err != nil || read.Code != http.StatusOK {
		t.Fatalf("the narrower reader's read answered %d %s", read.Code, read.Body.String())
	}
	if run.Answer.Labels != nil {
		t.Errorf("a reader without the company grant was handed company names: %v", *run.Answer.Labels)
	}

	cell := httptest.NewRecorder()
	routes.ExplainReportRunCell(cell, analyticsRequest(narrow, http.MethodPost, "/analytics/runs/x/cells/explain",
		`{"group":["`+company+`"]}`), *asked.RunId)
	out := explainBody(t, cell)
	if out.Labels != nil {
		t.Errorf("the cell handed a reader without the company grant company names: %v", *out.Labels)
	}
	if len(out.Rows) != 6 {
		t.Fatalf("the cell opened to %d records, want 6", len(out.Rows))
	}
	for _, row := range out.Rows {
		if row[derivationLabelColumn] != "Contoso deal" {
			t.Errorf("deal %v is labelled %v, want its name", row["id"], row[derivationLabelColumn])
		}
	}
}
