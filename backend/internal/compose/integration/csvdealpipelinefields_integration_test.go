// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// pipelineIDNamed is the id of the listed pipeline with that name.
func pipelineIDNamed(t *testing.T, e *apptest.AppEnv, name string) string {
	t.Helper()
	var pipelines struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	e.Call(t, http.MethodGet, "/v1/pipelines", nil, nil, &pipelines)
	for _, pipeline := range pipelines.Data {
		if pipeline.Name == name {
			return pipeline.ID
		}
	}
	t.Fatalf("no pipeline named %q", name)
	return ""
}

// A deal file's dry run and its commit skip the same rows for the same reason.
// The cause is a custom value for a field that does not apply to the pipeline.
func TestTheDryRunAndTheCommitSkipADealWhoseFieldDoesNotApplyToItsPipeline(t *testing.T) {
	e := setupCustomFieldImportApp(t)
	home, open, _, _ := seededStages(t, e)
	e.Call(t, http.MethodPost, "/v1/pipelines", AnyMap{"name": "Partners", "stages": []AnyMap{{"name": "Intake", "position": 1}}}, nil, nil)
	var field struct {
		ID         string `json:"id"`
		ColumnName string `json:"column_name"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/custom-fields", AnyMap{
		"object": "deal", "label": "Partner tier", "type": "text", "source": "manual",
	}, nil, &field); status != http.StatusCreated {
		t.Fatalf("creating the deal field → %d, want 201", status)
	}
	column := field.ColumnName
	header := "ref,name,pipeline,stage," + column

	first := header + "\n" +
		fmt.Sprintf("D-1,At home,%s,%s,gold\n", home, open) +
		"D-2,Elsewhere,Partners,Intake,\n"
	importDeals(t, e, map[string]string{"deals.csv": first}, dealImportParts(header))
	if status := e.Call(t, http.MethodPatch, "/v1/custom-fields/"+field.ID, AnyMap{
		"pipeline_ids": []string{pipelineIDNamed(t, e, home)},
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("narrowing the field to the seeded pipeline → %d, want 200", status)
	}

	second := header + "\n" +
		fmt.Sprintf("D-1,At home,%s,%s,silver\n", home, open) +
		"D-2,Elsewhere,Partners,Intake,gold\n" +
		"D-3,Born elsewhere,Partners,Intake,gold\n"
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, map[string]string{"deals.csv": second})).SourceRef, "crm", dealImportParts(header))
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)

	if committed.Disposition.Updated != 1 || committed.Disposition.Skipped != 2 || len(committed.Issues) != 2 {
		t.Fatalf("commit = %+v, issues %+v, want the home deal updated and the two others skipped", committed.Disposition, committed.Issues)
	}
	if p, c := preview.Disposition, committed.Disposition; p.Created != c.Created || p.Updated != c.Updated || p.Skipped != c.Skipped {
		t.Errorf("dry run = %+v, commit = %+v, want the same outcome per row", preview.Disposition, committed.Disposition)
	}
	for i, issue := range committed.Issues {
		if issue.Reason == "" || len(preview.Issues) != len(committed.Issues) || preview.Issues[i].Reason != issue.Reason {
			t.Errorf("issue %d: dry run %+v, commit %+v, want the same reason", i, preview.Issues, committed.Issues)
		}
	}
}
