// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestALeadWhoseLinkedInURLIsHeldIsSkippedInThePreviewAndTheCommit(t *testing.T) {
	e := setupImportApp(t)
	var held struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/leads", AnyMap{"full_name": "Held Lead", "email": "held@lead.example", "linkedin_url": "https://www.linkedin.com/in/held-lead", "source": "manual"}, nil, &held); status != http.StatusCreated {
		t.Fatalf("POST /v1/leads → %d, want 201", status)
	}
	if got := recordField(t, e, "/v1/leads", "email", "held@lead.example", "linkedin_url"); got != "https://www.linkedin.com/in/held-lead" {
		t.Fatalf("held lead linkedin_url = %q, want the profile it was created with", got)
	}
	profile, _ := uploadCSV(t, e, "lead", "Email,Name,LinkedIn\nother@lead.example,Other,https://www.linkedin.com/in/held-lead/\n")
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, map[string]string{"Email": "email", "Name": "full_name", "LinkedIn": "linkedin_url"})
	if preview := readImportReport(t, e, run.ID); preview.Disposition.Skipped != 1 {
		t.Fatalf("preview disposition = %+v, want the row skipped", preview.Disposition)
	}
	approveAndRead(t, e, run.ID)
	if report := readImportReport(t, e, run.ID); report.Disposition.Skipped != 1 || report.Disposition.Created != 0 {
		t.Fatalf("commit disposition = %+v, want the row skipped, as previewed", report.Disposition)
	}
}
