// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func stageBundle(t *testing.T, e *apptest.AppEnv, sourceRef, sourceName string, parts []AnyMap) importRunDTO {
	t.Helper()
	var run importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "source_ref": sourceRef, "source_name": sourceName, "parts": parts,
	}, nil, &run); status != http.StatusAccepted {
		t.Fatalf("stage → %d", status)
	}
	return run
}

func bundleParts() []AnyMap {
	return []AnyMap{
		{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{"ref": "ref", "email": "email", "name": "full_name"}},
		{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name"}},
	}
}

// approveAndRead commits a staged run and reads it back once it is terminal.
func approveAndRead(t *testing.T, e *apptest.AppEnv, runID string) {
	t.Helper()
	if status := importCall(t, e, runID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	var run importRunDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+runID, nil, nil, &run); status != http.StatusOK {
		t.Fatalf("read run → %d, want 200", status)
	}
	if run.Status != "complete" {
		t.Fatalf("run status = %q after approval, want complete", run.Status)
	}
}

// archiveOrder lists, oldest first, the types of the records one run landed
// that were since archived.
func archiveOrder(t *testing.T, e *apptest.AppEnv, runID string, entityTypes ...string) []string {
	t.Helper()
	rows, err := e.Pool.Query(context.Background(), `
		SELECT entity_type FROM audit_log
		 WHERE action = 'archive' AND entity_type = ANY($1)
		   AND entity_id IN (SELECT native_id FROM import_record_map WHERE import_run_id = $2)
		 ORDER BY occurred_at, id`, entityTypes, runID)
	if err != nil {
		t.Fatalf("reading the archive order: %v", err)
	}
	defer rows.Close()
	var order []string
	for rows.Next() {
		var entityType string
		if err := rows.Scan(&entityType); err != nil {
			t.Fatalf("reading the archive order: %v", err)
		}
		order = append(order, entityType)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the archive order: %v", err)
	}
	return order
}

func TestABundleLandsEveryPartAndAReUploadUpdatesInsteadOfDuplicating(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{
		"companies.csv": "ref,name\nCO-1,Acme Bundle GmbH\n",
		"contacts.csv":  "ref,email,name\nP-1,jo@acme-bundle.example,Jo Tran\n",
	}
	profile := uploadZip(t, e, "crm.zip", zipBytes(t, files))
	run := stageBundle(t, e, profile.SourceRef, "crm", bundleParts())
	var report importReportDTO
	e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report)
	if report.Disposition.Created != 2 || len(report.Parts) != 2 || report.Parts[0].Object != "company" {
		t.Fatalf("report = %+v, want 2 creates over two parts, companies first", report)
	}
	approveAndRead(t, e, run.ID)

	files["companies.csv"] = "ref,name\nCO-1,Acme Bundle AG\n"
	again := uploadZip(t, e, "crm.zip", zipBytes(t, files))
	second := stageBundle(t, e, again.SourceRef, "crm", bundleParts())
	e.Call(t, http.MethodGet, "/v1/imports/"+second.ID+"/report", nil, nil, &report)
	if report.Disposition.Created != 0 || report.Disposition.Updated != 1 || report.Disposition.Unchanged != 1 {
		t.Fatalf("re-upload report = %+v, want 1 update and 1 unchanged", report.Disposition)
	}
}

func TestTheSameRefUnderANewSourceNameIsANewRecord(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{"companies.csv": "ref,name\n1,Northwind One\n"}
	parts := []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name"}}}
	first := stageBundle(t, e, uploadZip(t, e, "a.zip", zipBytes(t, files)).SourceRef, "hubspot", parts)
	approveAndRead(t, e, first.ID)
	files["companies.csv"] = "ref,name\n1,Contoso Two\n"
	second := stageBundle(t, e, uploadZip(t, e, "b.zip", zipBytes(t, files)).SourceRef, "pipedrive", parts)
	var report importReportDTO
	e.Call(t, http.MethodGet, "/v1/imports/"+second.ID+"/report", nil, nil, &report)
	if report.Disposition.Created != 1 {
		t.Fatalf("report = %+v, want ref 1 under a new source name to create", report.Disposition)
	}
}

func TestUndoReversesContactsBeforeCompanies(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{
		"companies.csv": "ref,name\nCO-9,Undo Order GmbH\n",
		"contacts.csv":  "ref,email,name\nP-9,ann@undo-order.example,Ann\n",
	}
	run := stageBundle(t, e, uploadZip(t, e, "u.zip", zipBytes(t, files)).SourceRef, "u", bundleParts())
	approveAndRead(t, e, run.ID)
	var undone importRunDTO
	if status := importCall(t, e, run.ID, "undo", nil, &undone); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	order := archiveOrder(t, e, run.ID, "contact", "company")
	if len(order) != 2 || order[0] != "contact" {
		t.Fatalf("archive order %v, want the contact before the company", order)
	}
}
