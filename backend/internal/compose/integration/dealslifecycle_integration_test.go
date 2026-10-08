// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A deal's closing is one verb, advance, and a closed deal stays as it closed.
// The forecast reads only deals that can still land, the PATCH refuses the
// closing fields it cannot honour, and a stage from another pipeline is a 422.

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type dealRead struct {
	ID         string  `json:"id"`
	Version    int64   `json:"version"`
	Status     string  `json:"status"`
	ClosedAt   *string `json:"closed_at"`
	LostReason *string `json:"lost_reason"`
}

type faultBody struct {
	Code    string `json:"code"`
	Details struct {
		Errors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"errors"`
	} `json:"details"`
}

func (f faultBody) first() (field, code string) {
	if len(f.Details.Errors) == 0 {
		return "", f.Code
	}
	return f.Details.Errors[0].Field, f.Details.Errors[0].Code
}

func readDealStatus(t *testing.T, e *apptest.AppEnv, id string) dealRead {
	t.Helper()
	var d dealRead
	if status := e.Call(t, "GET", "/v1/deals/"+id, nil, nil, &d); status != http.StatusOK {
		t.Fatalf("reading the deal → %d", status)
	}
	return d
}

func TestALostDealIsNotInTheForecastsOpenPipeline(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)

	type readings struct {
		OpenMinor     int64 `json:"open_minor"`
		BestCaseMinor int64 `json:"best_case_minor"`
		EvidenceMinor int64 `json:"evidence_minor"`
		EligibleCount int   `json:"eligible_count"`
		PricedCount   int   `json:"priced_count"`
	}
	read := func() readings {
		var r readings
		if status := e.Call(t, "GET", "/v1/forecast?period=quarter", nil, nil, &r); status != http.StatusOK {
			t.Fatalf("forecast → %d", status)
		}
		return r
	}
	before := read()

	now := time.Now().UTC()
	quarterEnd := time.Date(now.Year(), ((now.Month()-1)/3+1)*3+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	var deal dealRead
	if status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": "Will be lost", "amount_minor": 222200, "currency": "EUR", "source": "manual",
		"pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"expected_close_date": quarterEnd.Format("2006-01-02"),
	}, nil, &deal); status != http.StatusCreated {
		t.Fatalf("create deal → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/deals/"+deal.ID, AnyMap{"forecast_category": "commit", "expected_close_date": quarterEnd.Format("2006-01-02")},
		map[string]string{"If-Match": strconv.FormatInt(deal.Version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("committing the deal → %d", status)
	}
	open := read()
	if open.OpenMinor != before.OpenMinor+222200 || open.EligibleCount != before.EligibleCount+1 {
		t.Fatalf("control: the open deal moved open_minor %d → %d and eligible %d → %d, want +222200 and +1",
			before.OpenMinor, open.OpenMinor, before.EligibleCount, open.EligibleCount)
	}

	if status := e.Call(t, "POST", "/v1/deals/"+deal.ID+"/advance",
		AnyMap{"to_stage_id": stages.Lost, "lost_reason": "went with a competitor on price"}, nil, nil); status != http.StatusOK {
		t.Fatalf("losing the deal → %d", status)
	}
	if lost := read(); lost != before {
		t.Errorf("after the deal was lost the forecast reads %+v, want the figures from before it existed %+v", lost, before)
	}
}

func TestAPatchNamingTheClosingFieldsIsRefusedNotIgnored(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)
	before := readDealStatus(t, e, dealID)

	for _, body := range []AnyMap{
		{"status": "lost", "lost_reason": "went with a competitor on price"},
		{"fx_rate_to_base": "1.1"},
		{"fx_rate_date": "2026-01-01"},
	} {
		var fault faultBody
		status := e.Call(t, "PATCH", "/v1/deals/"+dealID, body,
			map[string]string{"If-Match": strconv.FormatInt(before.Version, 10)}, &fault)
		if field, _ := fault.first(); status != http.StatusUnprocessableEntity || field == "" {
			t.Errorf("PATCH %v → %d %+v, want a 422 naming the field and pointing at advance", body, status, fault)
		}
	}
	if after := readDealStatus(t, e, dealID); after.Version != before.Version || after.Status != "open" {
		t.Errorf("the refused patches changed the deal: %+v → %+v", before, after)
	}
}

func TestADealCannotBeBornInAnotherPipelinesStage(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	var other struct {
		Stages []struct {
			ID string `json:"id"`
		} `json:"stages"`
	}
	if status := e.Call(t, "POST", "/v1/pipelines", AnyMap{
		"name": "Partnerships", "stages": []AnyMap{{"name": "Scout", "position": 1}},
	}, nil, &other); status != http.StatusCreated || len(other.Stages) == 0 {
		t.Fatalf("create the second pipeline → %d %+v", status, other)
	}

	var fault faultBody
	status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": "Wrong stage", "source": "manual",
		"pipeline_id": stages.PipelineID, "stage_id": other.Stages[0].ID,
	}, nil, &fault)
	if field, code := fault.first(); status != http.StatusUnprocessableEntity || field != "stage_id" || code != "stage_not_in_pipeline" {
		t.Errorf("a stage of another pipeline → %d %+v, want 422 stage_not_in_pipeline on stage_id", status, fault)
	}
}

func TestAClosedDealCannotBeClosedAgainByMovingToItsOwnStage(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)

	won := apptest.CreateOpenDeal(t, e, stages)
	if status := e.Call(t, "POST", "/v1/deals/"+won+"/advance",
		AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}, nil, nil); status != http.StatusOK {
		t.Fatalf("winning the deal → %d", status)
	}
	closed := readDealStatus(t, e, won)
	var fault faultBody
	if status := e.Call(t, "POST", "/v1/deals/"+won+"/advance",
		AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "purchase_order"}, nil, &fault); status != http.StatusUnprocessableEntity {
		t.Errorf("winning an already won deal → %d %+v, want 422", status, fault)
	}
	if after := readDealStatus(t, e, won); after.ClosedAt == nil || closed.ClosedAt == nil || *after.ClosedAt != *closed.ClosedAt || after.Version != closed.Version {
		t.Errorf("a repeated win moved the close: %+v → %+v", closed, after)
	}

	var second dealRead
	if status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": "Second", "source": "manual", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
	}, nil, &second); status != http.StatusCreated {
		t.Fatalf("create the second deal → %d", status)
	}
	lost := second.ID
	if status := e.Call(t, "POST", "/v1/deals/"+lost+"/advance",
		AnyMap{"to_stage_id": stages.Lost, "lost_reason": "price"}, nil, nil); status != http.StatusOK {
		t.Fatalf("losing the deal → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/deals/"+lost+"/advance",
		AnyMap{"to_stage_id": stages.Lost, "lost_reason": "a different reason"}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("losing an already lost deal → %d, want 422", status)
	}
	if after := readDealStatus(t, e, lost); after.LostReason == nil || *after.LostReason != "price" {
		t.Errorf("a repeated loss replaced the reason: %+v", after)
	}
}

func TestALostDealNeedsAReasonThatSaysSomething(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)

	for _, reason := range []string{"", "   ", "\t\n"} {
		var fault faultBody
		status := e.Call(t, "POST", "/v1/deals/"+dealID+"/advance",
			AnyMap{"to_stage_id": stages.Lost, "lost_reason": reason}, nil, &fault)
		if _, code := fault.first(); status != http.StatusUnprocessableEntity || code != "lost_reason_required" {
			t.Errorf("lost_reason %q → %d %+v, want 422 lost_reason_required", reason, status, fault)
		}
	}
	if status := e.Call(t, "POST", "/v1/deals/"+dealID+"/advance",
		AnyMap{"to_stage_id": stages.Lost, "lost_reason": "  too expensive  "}, nil, nil); status != http.StatusOK {
		t.Fatalf("a real reason → %d, want 200", status)
	}
	if got := readDealStatus(t, e, dealID); got.LostReason == nil || *got.LostReason != "too expensive" {
		t.Errorf("the stored reason is %v, want it trimmed to \"too expensive\"", got.LostReason)
	}
}
