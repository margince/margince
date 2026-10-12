// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestPreviewImportStagesOnePartPerFile(t *testing.T) {
	var staged crmcontracts.CreateImportRunRequest
	_, err := previewImport{imports: recordingImports{staged: &staged, profile: twoPartProfile()}}.Handle(context.Background(), json.RawMessage(`{
		"source_name":"crm","on_broken_link":"drop_link",
		"files":[{"name":"accounts.csv","object":"company","csv":"ref,name\nC1,Acme\n"},
		         {"name":"people.csv","object":"contact","csv":"ref,email\nP1,jo@acme.test\n"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if staged.Parts == nil || len(*staged.Parts) != 2 || *staged.SourceName != "crm" {
		t.Fatalf("staged = %+v", staged)
	}
	if staged.OnBrokenLink == nil || *staged.OnBrokenLink != crmcontracts.ImportOnBrokenLinkDropLink {
		t.Errorf("on_broken_link was not carried: %+v", staged.OnBrokenLink)
	}
	for _, part := range *staged.Parts {
		if part.Mapping["ref"] != "ref" {
			t.Errorf("part %s staged mapping %v, want the proposal it covered", part.File, part.Mapping)
		}
	}
}

// One file is still a ZIP part. The seam stores the files as a ZIP, and a run
// reads parts from a ZIP and a plain mapping from a CSV.
func TestPreviewImportStagesASingleFileAsAPart(t *testing.T) {
	var staged crmcontracts.CreateImportRunRequest
	_, err := previewImport{imports: recordingImports{staged: &staged, profile: twoPartProfile()}}.Handle(context.Background(), json.RawMessage(`{
		"files":[{"name":"accounts.csv","object":"company","csv":"ref,name\nC1,Acme\n"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if staged.Parts == nil || len(*staged.Parts) != 1 || staged.Mapping != nil || staged.Object != nil {
		t.Fatalf("staged = %+v, want one part and no single-file mapping", staged)
	}
}

func TestPreviewImportRefusesFilesThatTogetherPassTheLimit(t *testing.T) {
	big := strings.Repeat("a", maxImportCSVBytes/2+1)
	args := fmt.Sprintf(`{"files":[{"name":"a.csv","object":"lead","csv":%q},{"name":"b.csv","object":"company","csv":%q}]}`, big, big)
	var stored bool
	if _, err := (previewImport{imports: recordingImports{stored: &stored}}).Handle(context.Background(), json.RawMessage(args)); err == nil {
		t.Fatal("two files over the limit together must be refused")
	}
	if stored {
		t.Error("files over the limit reached the object store")
	}
}

func TestPreviewImportRefusesTwoFilesOfOneType(t *testing.T) {
	_, err := previewImport{imports: recordingImports{profile: twoPartProfile()}}.Handle(context.Background(), json.RawMessage(`{
		"files":[{"name":"a.csv","object":"company","csv":"name\nA\n"},{"name":"b.csv","object":"company","csv":"name\nB\n"}]}`))
	if err == nil || !strings.Contains(err.Error(), "a.csv") || !strings.Contains(err.Error(), "b.csv") {
		t.Fatalf("two company files answered %v, want a refusal naming both", err)
	}
}

func TestPreviewImportRefusesFilesBesideASingleCSV(t *testing.T) {
	_, err := previewImport{imports: recordingImports{}}.Handle(context.Background(), json.RawMessage(`{
		"object":"company","csv":"name\nA\n","files":[{"name":"a.csv","object":"company","csv":"name\nA\n"}]}`))
	if err == nil {
		t.Fatal("a call naming both shapes was accepted")
	}
}

// A file whose proposal places only some columns is refused as a single CSV
// is. The stored ZIP goes with the refusal.
func TestAFilesProposalThatPlacesOnlySomeColumnsIsRefusedAndDiscarded(t *testing.T) {
	var discarded []string
	profile := twoPartProfile()
	(*profile.Parts)[0].Columns = append((*profile.Parts)[0].Columns, crmcontracts.ImportColumn{Header: "Band"})
	_, err := previewImport{imports: recordingImports{profile: profile, discarded: &discarded}}.Handle(context.Background(), json.RawMessage(`{
		"files":[{"name":"accounts.csv","object":"company","csv":"ref,name,Band\nC1,Acme,x\n"}]}`))
	if err == nil || !strings.Contains(err.Error(), `"Band"`) || !strings.Contains(err.Error(), "accounts.csv") {
		t.Fatalf("answered %v, want a refusal naming the file and its unplaced column", err)
	}
	if len(discarded) != 1 {
		t.Errorf("discarded = %v, want the stored ZIP", discarded)
	}
}

func TestReadImportReportAnswersTheRunStateBeforeAReportExists(t *testing.T) {
	run := crmcontracts.ImportRun{Status: "validating"}
	out, err := readImportReport{imports: recordingImports{run: run}}.Handle(context.Background(), json.RawMessage(`{"run_id":"`+ids.NewV7().String()+`"}`))
	if err != nil {
		t.Fatalf("a run with no report yet answered %v; an agent polling after commit_import would read a dead end", err)
	}
	var got ImportReportResult
	if err := json.Unmarshal(out, &got); err != nil || got.State != "validating" || got.Report != nil {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestReadImportReportCarriesTheRunsPartsAndReport(t *testing.T) {
	parts := []crmcontracts.ImportRunPart{{File: "companies.csv", Object: importObjectCompany, RowCount: 2}}
	run := crmcontracts.ImportRun{Status: "complete", Parts: &parts, Checkpoint: 2}
	report := crmcontracts.ImportRunReport{RowsRead: 2}
	out, err := readImportReport{imports: recordingImports{run: run, report: &report}}.Handle(context.Background(), json.RawMessage(`{"run_id":"`+ids.NewV7().String()+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got ImportReportResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.State != "complete" || got.Checkpoint != 2 || len(got.Parts) != 1 || got.Report == nil || got.Report.RowsRead != 2 {
		t.Fatalf("got %+v", got)
	}
}

// twoPartProfile is what the seam answers for a company file and a contact
// file, each proposal placing every column.
func twoPartProfile() crmcontracts.ImportSourceProfile {
	company, contact := crmcontracts.ImportObject(importObjectCompany), crmcontracts.ImportObject(importObjectContact)
	parts := []crmcontracts.ImportPartProfile{
		{
			File: "companies.csv", Object: &company,
			Columns:           []crmcontracts.ImportColumn{{Header: "ref"}, {Header: "name"}},
			SuggestedMappings: map[string]map[string]string{importObjectCompany: {"ref": "ref", "name": "display_name"}},
		},
		{
			File: "contacts.csv", Object: &contact,
			Columns:           []crmcontracts.ImportColumn{{Header: "ref"}, {Header: "email"}},
			SuggestedMappings: map[string]map[string]string{importObjectContact: {"ref": "ref", "email": "email"}},
		},
	}
	return crmcontracts.ImportSourceProfile{SourceRef: "ws/import-source/bundle", Parts: &parts}
}
