// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestADealsFileIsAPartThatStagesAndTestRuns(t *testing.T) {
	e := setupImportApp(t)
	profile := uploadZip(t, e, "crm.zip", zipBytes(t, map[string]string{
		"deals.csv":     "ref,name,company_ref\nD-1,Pumps for Acme,CO-1\n",
		"companies.csv": "ref,name\nCO-1,Acme Deals GmbH\n",
	}))
	if len(profile.Parts) != 2 || profile.Parts[1].Object != "deal" || len(profile.Ignored) != 0 {
		t.Fatalf("profile = %+v, want companies then deals and nothing ignored", profile)
	}
	run := stageBundle(t, e, profile.SourceRef, "crm", []AnyMap{
		{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name"}},
		{"file": "deals.csv", "object": "deal", "mapping": AnyMap{"ref": "ref", "name": "name", "company_ref": "company_ref"}},
	})
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d", status)
	}
	if report.Disposition.Created != 2 || len(report.Parts) != 2 || report.Parts[1].Object != "deal" || len(report.Issues) != 0 {
		t.Fatalf("report = %+v, want the deal predicted as a create whose company link finds the ZIP's company", report)
	}
}

func TestADealsFileWithoutARefColumnIsRefusedAtStaging(t *testing.T) {
	e := setupImportApp(t)
	profile := uploadZip(t, e, "crm.zip", zipBytes(t, map[string]string{"deals.csv": "name\nPumps\n"}))
	status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "source_ref": profile.SourceRef, "source_name": "crm",
		"parts": []AnyMap{{"file": "deals.csv", "object": "deal", "mapping": AnyMap{"name": "name"}}},
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("stage → %d, want 422: two deals may share a name, so a deal row needs its ref", status)
	}
}

// seededStageNames is the seeded pipeline's name and the name of one open and
// one won stage on it.
func seededStageNames(t *testing.T, e *apptest.AppEnv) (pipeline, open, won string) {
	t.Helper()
	var pipelines struct {
		Data []struct {
			Name   string `json:"name"`
			Stages []struct {
				Name     string `json:"name"`
				Semantic string `json:"semantic"`
			} `json:"stages"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/pipelines", nil, nil, &pipelines); status != http.StatusOK || len(pipelines.Data) == 0 {
		t.Fatalf("list pipelines → %d, %+v", status, pipelines)
	}
	for _, s := range pipelines.Data[0].Stages {
		switch {
		case s.Semantic == "open" && open == "":
			open = s.Name
		case s.Semantic == "won" && won == "":
			won = s.Name
		}
	}
	return pipelines.Data[0].Name, open, won
}

func TestADealsTestRunReportsAStageOutsideItsPipelineAndAnOpenDealClosingInThePast(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, won := seededStageNames(t, e)
	deals := "ref,name,pipeline,stage,amount,currency,close_date\n" +
		fmt.Sprintf("D-1,Open,%s,%s,12000.50,EUR,2099-01-31\n", strings.ToUpper(pipeline), open) +
		fmt.Sprintf("D-2,Won last year,%s,%s,500,EUR,2025-06-30\n", pipeline, won) +
		fmt.Sprintf("D-3,Nowhere,%s,Closing,,,\n", pipeline) +
		fmt.Sprintf("D-4,Late,%s,%s,,,2020-01-01\n", pipeline, open)
	profile := uploadZip(t, e, "crm.zip", zipBytes(t, map[string]string{"deals.csv": deals}))
	mapping := AnyMap{
		"ref": "ref", "name": "name", "pipeline": "pipeline", "stage": "stage",
		"amount": "amount", "currency": "currency", "close_date": "close_date",
	}
	run := stageBundle(t, e, profile.SourceRef, "crm", []AnyMap{{"file": "deals.csv", "object": "deal", "mapping": mapping}})
	var report importReportDTO
	e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report)
	if report.Disposition.Created != 2 || report.Disposition.Skipped != 2 {
		t.Fatalf("disposition = %+v, want the open and the won deal to land and two rows skipped", report.Disposition)
	}
	byLine := map[int]importIssueDTO{}
	for _, issue := range report.Issues {
		byLine[issue.Line] = issue
	}
	if got := byLine[4]; got.Kind != "invalid_value" || got.Column != "stage" || !strings.Contains(got.Reason, pipeline) {
		t.Errorf("line 4 = %+v, want the stage refused naming the pipeline it is not in", got)
	}
	if got := byLine[5]; got.Kind != "invalid_value" || got.Column != "close_date" {
		t.Errorf("line 5 = %+v, want an open deal's past close date refused", got)
	}
}
