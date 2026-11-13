// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

const inputChecks = "/v1/forecast/assurance/exceptions"

// switchOff turns "Show individual performance" off.
func switchOff(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	mustCall(t, e, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, http.StatusOK, nil)
}

// wantFloored fails unless path answers 403 individual_performance_off.
func wantFloored(t *testing.T, e *apptest.AppEnv, path string) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, http.MethodGet, path, nil, nil, &problem); status != http.StatusForbidden || problem.Code != "individual_performance_off" {
		t.Errorf("with the switch off, %s = %d %q, want 403 individual_performance_off", path, status, problem.Code)
	}
}

// wantToolFloored fails unless the tool is refused for the switch.
func wantToolFloored(t *testing.T, mcp *apptest.MCPClient, tool string) {
	t.Helper()
	if refusal := mcp.CallRefused(t, tool, nil); !strings.Contains(strings.ToLower(refusal), "individual performance") {
		t.Errorf("with the switch off, %s was refused with %q, want the switch's refusal", tool, refusal)
	}
}

// openDealOf opens one deal owned by owner and answers it.
func openDealOf(t *testing.T, e *apptest.AppEnv, owner string) string {
	t.Helper()
	stages := apptest.DiscoverSeededPipeline(t, e)
	return createdRecord(t, e, "/v1/deals", AnyMap{
		"name": "Checked deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual", "owner_id": owner,
	})
}

// A preview that raises nothing still counts the deals it checked. With one
// colleague's deal and none of the reader's, that count is the colleague's.
func TestIndividualPerformanceOffRefusesAPreviewOfOneColleaguesDeals(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithAssuranceRun(nil))
	_, colleague, _ := twoSellerTeam(t, e, "preview-eligible")
	openDealOf(t, e, colleague)
	var preview struct {
		EligibleDeals int               `json:"eligible_deals"`
		Findings      []json.RawMessage `json:"findings"`
	}
	mustCall(t, e, http.MethodGet, "/v1/forecast/assurance/preview", nil, http.StatusOK, &preview)
	if preview.EligibleDeals != 1 || len(preview.Findings) != 0 {
		t.Fatalf("with the switch on, the preview counts %d deals and %d findings, want the one deal and no finding", preview.EligibleDeals, len(preview.Findings))
	}
	switchOff(t, e)
	wantFloored(t, e, "/v1/forecast/assurance/preview")
}

// A call's finding counts the deals that moved across its scope, whoever made
// the call. The reader's own call on a team of the reader and one colleague
// shows that colleague's moves, though the whole list is safe.
func TestIndividualPerformanceOffRefusesACallFindingOfOneColleaguesMoves(t *testing.T) {
	d := newTwoDoors(t, "call-finding-floor", "read")
	d.e.DescribeCompany(t)
	me := signedInUser(t, d.e)
	colleagues := addColleagues(t, d.e, "call-finding-floor", 3)
	team := teamWithMembers(t, d.e, me, colleagues[0])
	var moved string
	for index, colleague := range colleagues {
		deal := openDealOf(t, d.e, colleague)
		raiseNoNextStep(t, d.e, deal, colleague, 100_000)
		if index == 0 {
			moved = deal
		}
	}
	var call struct {
		ID string `json:"id"`
	}
	mustCall(t, d.e, http.MethodPost, "/v1/forecast/calls", AnyMap{
		"amount_minor": 500_000, "currency": "EUR", "scope_kind": "team", "scope_id": team,
	}, http.StatusCreated, &call)
	raiseFinding(t, d.e, assurance.Finding{
		Type: assurance.TypeForecastCallMoved, SubjectKind: assurance.SubjectKindForecastCall, SubjectID: call.ID,
		Severity: "medium", Claim: map[string]any{"scope_kind": "team", "scope_id": team},
		Observed: map[string]any{"comparison": "moved", "changed_deals": 1, "deal_ids": []string{moved}, "weighted_change_minor": -40_000},
	}, me)
	var listed struct {
		Data []struct {
			SubjectKind string `json:"subject_kind"`
		} `json:"data"`
	}
	mustCall(t, d.e, http.MethodGet, inputChecks, nil, http.StatusOK, &listed)
	if len(listed.Data) != 4 {
		t.Fatalf("with the switch on, the list holds %d findings, want the three deals' and the call's", len(listed.Data))
	}
	switchOff(t, d.e)
	wantFloored(t, d.e, inputChecks)
	wantToolFloored(t, d.mcp, "list_input_checks")
}

// The whole list is safe with three colleagues. A figure drawn from one of them
// alone is still that colleague's: a finding type, or a type in one currency.
func TestIndividualPerformanceOffRefusesAFindingGroupOfOneColleague(t *testing.T) {
	alone := map[string]func(deal string, amount int64) assurance.Finding{
		"a type": func(deal string, amount int64) assurance.Finding {
			return assurance.Finding{Type: assurance.TypeClosePast, SubjectID: deal, Severity: "high", AffectedMinor: &amount, Currency: "EUR"}
		},
		"a currency": func(deal string, amount int64) assurance.Finding {
			return assurance.Finding{Type: assurance.TypeNoNextStep, SubjectID: deal, Severity: "medium", AffectedMinor: &amount, Currency: "USD"}
		},
	}
	for name, finding := range alone {
		t.Run(name, func(t *testing.T) {
			e := apptest.SetupApp(t)
			e.BootstrapWorkspace(t)
			e.DescribeCompany(t)
			colleagues := addColleagues(t, e, "group-floor", 3)
			for _, colleague := range colleagues {
				raiseNoNextStep(t, e, openDealOf(t, e, colleague), colleague, 100_000)
			}
			last := colleagues[2]
			raiseFinding(t, e, finding(openDealOf(t, e, last), 70_000), last)
			mustCall(t, e, http.MethodGet, inputChecks, nil, http.StatusOK, nil)
			switchOff(t, e)
			wantFloored(t, e, inputChecks)
		})
	}
}

// The last check's counts are made of colleagues' deals. With one colleague's
// deal and none of the reader's, the count is the colleague's, on the route
// and on the tool.
func TestIndividualPerformanceOffRefusesACheckRunOfOneColleaguesDeals(t *testing.T) {
	d := newTwoDoors(t, "run-floor", "read")
	d.e.DescribeCompany(t)
	colleague := addColleagues(t, d.e, "run-floor", 1)[0]
	openDealOf(t, d.e, colleague)
	scanner := assurance.NewScanner(assurance.NewStore(compose.InstallationDB(d.e.Pool)),
		compose.AssuranceSubjects, compose.AssuranceCoverage, compose.AssuranceConfig())
	if _, err := scanner.Scan(apptest.AdminContext(t, d.e, AdminPerms), clocktest.Now(t), nil); err != nil {
		t.Fatalf("running the check: %v", err)
	}
	var run struct {
		EligibleDeals int `json:"eligible_deals"`
	}
	mustCall(t, d.e, http.MethodGet, "/v1/forecast/assurance", nil, http.StatusOK, &run)
	if run.EligibleDeals != 1 {
		t.Fatalf("with the switch on, the run checked %d deals, want the colleague's one", run.EligibleDeals)
	}
	d.tool(t, "forecast_input_checks", nil)
	switchOff(t, d.e)
	wantFloored(t, d.e, "/v1/forecast/assurance")
	wantToolFloored(t, d.mcp, "forecast_input_checks")
}

// A stage's recorded chance is drawn from the deals that passed through it.
// With only a colleague's deal behind it, it is the colleague's.
func TestIndividualPerformanceOffRefusesStageChancesOfOneColleaguesDeals(t *testing.T) {
	e := apptest.SetupApp(t)
	_, colleague, _ := twoSellerTeam(t, e, "chances-floor")
	deal := openDealOf(t, e, colleague)
	if status := e.Call(t, http.MethodPost, "/v1/deals/"+deal+"/advance", AnyMap{"to_stage_id": openStages(t, e)[1]}, nil, nil); status != http.StatusOK {
		t.Fatalf("moving the deal on = %d", status)
	}
	// The evidence counts moves before today, so the move is dated yesterday.
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE deal_stage_history SET changed_at = changed_at - interval '2 days' WHERE deal_id = $1`, deal); err != nil {
		t.Fatalf("dating the move: %v", err)
	}
	mustCall(t, e, http.MethodGet, "/v1/forecast/stage-chances", nil, http.StatusOK, nil)
	switchOff(t, e)
	wantFloored(t, e, "/v1/forecast/stage-chances")
}
