// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// Two pasted files land as one run through the seam the upload uses. The
// contact links to the other file's company by its ref. read_import answers the
// finished run with both parts and their counts.
func TestAnAgentImportsTwoLinkedFilesAsOneRun(t *testing.T) {
	d := newTwoDoorsWith(t, "import-files-tool",
		[]compose.Option{compose.WithImportCommitRunner(compose.RunImportInline)}, "read", "write")
	var preview struct {
		Run struct {
			RunID string `json:"run_id"`
			State string `json:"state"`
		} `json:"run"`
		Files []struct {
			Name string `json:"name"`
			Part string `json:"part"`
		} `json:"files"`
	}
	decode(t, d.tool(t, "preview_import", map[string]any{
		"source_name": "old-crm",
		"files": []map[string]any{
			{
				"name": "accounts.csv", "object": "company", "csv": "ref,name\nCO-1,Filesby GmbH\n",
				"mapping": map[string]string{"ref": "ref", "name": "display_name"},
			},
			{
				"name": "people.csv", "object": "contact", "csv": "ref,email,company\nP-1,jo@filesby.example,CO-1\n",
				"mapping": map[string]string{"ref": "ref", "email": "email", "company": "company_ref"},
			},
		},
	}), &preview)
	if preview.Run.State != "awaiting_approval" || len(preview.Files) != 2 || preview.Files[0].Part != "companies.csv" {
		t.Fatalf("the preview answered %+v, want a run awaiting approval with both files", preview)
	}

	d.tool(t, "commit_import", map[string]any{"run_id": preview.Run.RunID})
	var read struct {
		State  string            `json:"state"`
		Parts  []json.RawMessage `json:"parts"`
		Report *struct {
			Disposition struct {
				Created int `json:"created"`
			} `json:"disposition"`
			Issues []json.RawMessage `json:"issues"`
		} `json:"report"`
	}
	decode(t, d.tool(t, "read_import", map[string]any{"view": "report", "run_id": preview.Run.RunID}), &read)
	if read.State != "complete" || len(read.Parts) != 2 || read.Report == nil ||
		read.Report.Disposition.Created != 2 || len(read.Report.Issues) != 0 {
		t.Fatalf("the finished run reads %+v, want both rows created and no issues", read)
	}
}

//craft:ignore naked-any the destination is whichever answer shape the test reads
func decode(t *testing.T, raw json.RawMessage, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
}
