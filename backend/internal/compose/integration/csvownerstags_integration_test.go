// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// importAdminEmail is the address apptest.BootstrapWorkspace signs the admin in
// with; AppEnv carries no accessor for it.
const importAdminEmail = "ada@example.com"

func tagCount(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM tag`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// companyTags is the named company's tag names, sorted.
func companyTags(t *testing.T, e *apptest.AppEnv, displayName string) []string {
	t.Helper()
	id := recordField(t, e, "/v1/companies", "display_name", displayName, "id")
	var onRecord struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/records/company/"+id+"/tags", nil, nil, &onRecord); status != http.StatusOK {
		t.Fatalf("GET the company's tags → %d, want 200", status)
	}
	names := make([]string, 0, len(onRecord.Data))
	for _, tag := range onRecord.Data {
		names = append(names, tag.Name)
	}
	slices.Sort(names)
	return names
}

func TestOwnersAndTagsLandAndTheTestRunNamesNewTagsAndUnknownOwners(t *testing.T) {
	e := setupImportApp(t)
	csv := "Name,Owner,Tags\nTagged GmbH," + importAdminEmail + ",Partner; DACH\nOrphan AG,mai@old-crm.example,DACH\n"
	tagsBefore := tagCount(t, e)
	profile, _ := uploadCSV(t, e, "company", csv)
	run, _ := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "Owner": "owner_email", "Tags": "tags"})
	var report importReportDTO
	e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report)
	if !hasIssue(report, "new_tag", "Partner") || !hasIssue(report, "new_tag", "DACH") || !hasIssue(report, "unknown_owner", "mai@old-crm.example") {
		t.Fatalf("issues = %+v", report.Issues)
	}
	if tagCount(t, e) != tagsBefore {
		t.Fatal("the test run created a tag; it writes nothing")
	}
	approveAndRead(t, e, run.ID)
	if tags := companyTags(t, e, "Tagged GmbH"); !slices.Equal(tags, []string{"DACH", "Partner"}) {
		t.Fatalf("tags = %v", tags)
	}
	if tags := companyTags(t, e, "Orphan AG"); !slices.Equal(tags, []string{"DACH"}) {
		t.Fatalf("Orphan AG tags = %v, want the one DACH tag the run created once", tags)
	}
	if got := tagCount(t, e); got != tagsBefore+2 {
		t.Fatalf("tag count = %d, want %d: each new name created once", got, tagsBefore+2)
	}
}

func TestAReimportAddsTagsButNeverRemovesOne(t *testing.T) {
	e := setupImportApp(t)
	first, _ := uploadCSV(t, e, "company", "Name,Tags\nKeep GmbH,Partner\n")
	run, _ := createRunWithMapping(t, e, "company", first.SourceRef, map[string]string{"Name": "display_name", "Tags": "tags"})
	approveAndRead(t, e, run.ID)

	again, _ := uploadCSV(t, e, "company", "Name,Tags\nKeep GmbH,DACH\n")
	second, _ := createRunWithMapping(t, e, "company", again.SourceRef, map[string]string{"Name": "display_name", "Tags": "tags"})
	approveAndRead(t, e, second.ID)
	if tags := companyTags(t, e, "Keep GmbH"); !slices.Equal(tags, []string{"DACH", "Partner"}) {
		t.Fatalf("tags = %v, want the new one added and the old one kept", tags)
	}
}
