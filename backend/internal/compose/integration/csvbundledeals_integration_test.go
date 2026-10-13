// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
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
