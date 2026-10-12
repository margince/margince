// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func partsRequest(parts ...crmcontracts.ImportPartRequest) crmcontracts.CreateImportRunRequest {
	name, file := "pipedrive-export", "pipedrive-export.zip"
	return crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnector("csv"),
		SourceRef: "ws/import/x", Parts: &parts, SourceName: &name, FileName: &file,
	}
}

func TestAPartsRequestBecomesOneMappingPerFile(t *testing.T) {
	m, err := stagedMappingFrom(partsRequest(
		crmcontracts.ImportPartRequest{File: "contacts.csv", Object: crmcontracts.ImportObjectContact, Mapping: map[string]string{"ref": "ref", "email": "email"}},
		crmcontracts.ImportPartRequest{File: "companies.csv", Object: crmcontracts.ImportObjectCompany, Mapping: map[string]string{"ref": "ref", "name": "display_name"}},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 2 || m.SourceName != "pipedrive-export" || m.OnBrokenLink != "skip_row" || m.FileName != "pipedrive-export.zip" {
		t.Fatalf("mapping = %+v", m)
	}
	if m.Parts[0].SourceKey != "ref" {
		t.Fatalf("a mapped ref column is the part's source key, got %q", m.Parts[0].SourceKey)
	}
	if got := m.PartList(); len(got) != 2 || got[1].File != "companies.csv" {
		t.Fatalf("a parts mapping lists its own parts, got %+v", got)
	}
}

func TestAPartsRequestNamingOneTypeTwiceIsRefused(t *testing.T) {
	part := crmcontracts.ImportPartRequest{File: "a.csv", Object: crmcontracts.ImportObjectLead, Mapping: map[string]string{"email": "email"}}
	second := part
	second.File = "b.csv"
	_, err := stagedMappingFrom(partsRequest(part, second))
	var v *httperr.DetailedError
	if !errors.As(err, &v) || v.Fields[0].Code != "duplicate_object" {
		t.Fatalf("err = %v, want a duplicate_object refusal", err)
	}
	if msg := v.Fields[0].Message; !strings.Contains(msg, "a.csv") || !strings.Contains(msg, "b.csv") {
		t.Fatalf("refusal %q must name both files", msg)
	}
}

func TestARequestWithBothShapesIsRefused(t *testing.T) {
	req := partsRequest(crmcontracts.ImportPartRequest{File: "a.csv", Object: crmcontracts.ImportObjectLead, Mapping: map[string]string{"email": "email"}})
	obj := crmcontracts.ImportObjectLead
	req.Object = &obj
	if _, err := stagedMappingFrom(req); err == nil {
		t.Fatal("object+mapping and parts together must be refused")
	}
}

func TestARequestWithNeitherShapeIsRefused(t *testing.T) {
	_, err := stagedMappingFrom(crmcontracts.CreateImportRunRequest{Connector: "csv", SourceRef: "ws/import/x"})
	var v *httperr.DetailedError
	if !errors.As(err, &v) || v.Fields[0].Code != "one_shape" {
		t.Fatalf("err = %v, want a one_shape refusal", err)
	}
}

func TestASourceNameOutsideTheContractPatternIsRefused(t *testing.T) {
	req := partsRequest(crmcontracts.ImportPartRequest{File: "a.csv", Object: crmcontracts.ImportObjectLead, Mapping: map[string]string{"email": "email"}})
	bad := "HubSpot export (2)"
	req.SourceName = &bad
	_, err := stagedMappingFrom(req)
	var v *httperr.DetailedError
	if !errors.As(err, &v) || v.Fields[0].Field != "source_name" {
		t.Fatalf("err = %v, want a source_name refusal", err)
	}
}

func TestAPartsRequestKeepsAChosenBrokenLinkPolicy(t *testing.T) {
	req := partsRequest(crmcontracts.ImportPartRequest{File: "a.csv", Object: crmcontracts.ImportObjectLead, Mapping: map[string]string{"email": "email"}})
	policy := crmcontracts.ImportOnBrokenLinkDropLink
	req.OnBrokenLink = &policy
	m, err := stagedMappingFrom(req)
	if err != nil || m.OnBrokenLink != "drop_link" {
		t.Fatalf("m = %+v, err = %v", m, err)
	}
	unknown := crmcontracts.ImportOnBrokenLink("guess")
	req.OnBrokenLink = &unknown
	if _, err := stagedMappingFrom(req); err == nil {
		t.Fatal("a broken-link policy outside the enum must be refused")
	}
}

func TestASingleCSVRequestStillMapsAsToday(t *testing.T) {
	obj, mapping := crmcontracts.ImportObjectLead, map[string]string{"Email": "email"}
	m, err := stagedMappingFrom(crmcontracts.CreateImportRunRequest{
		Connector: "csv", SourceRef: "ws/import/x", Object: &obj, Mapping: &mapping,
	})
	if err != nil || m.Object != migration.ObjectLead || m.SourceKey != "Email" || len(m.PartList()) != 1 {
		t.Fatalf("m = %+v, err = %v", m, err)
	}
}

func TestASingleCSVRequestMappingRefRecordsItAsTheSourceKey(t *testing.T) {
	obj, mapping := crmcontracts.ImportObjectCompany, map[string]string{"Name": "display_name", "Company ID": "ref"}
	m, err := stagedMappingFrom(crmcontracts.CreateImportRunRequest{
		Connector: "csv", SourceRef: "ws/import/x", Object: &obj, Mapping: &mapping,
	})
	if err != nil || m.SourceKey != "Company ID" {
		t.Fatalf("m = %+v, err = %v", m, err)
	}
}

func zipRun() migration.Run {
	return migration.Run{
		Status: migration.StatusAwaitingApproval,
		Mapping: &migration.RunMapping{FileName: "export.zip", SourceName: "export", Parts: []migration.PartMapping{
			{File: "companies.csv", Object: migration.ObjectCompany},
			{File: "contacts.csv", Object: migration.ObjectContact},
		}},
		Report: &migration.Report{Objects: []migration.ObjectReport{
			{Object: migration.ObjectCompany, File: "companies.csv", MirrorCount: 2, WillCreate: 2},
			{
				Object: migration.ObjectContact, File: "contacts.csv", MirrorCount: 3, WillCreate: 2,
				Skipped:  []migration.SkippedRow{{ExternalID: "P-3", Line: 4, Reason: "no such company", Kind: migration.ProblemBrokenLink, Column: "company_ref", Value: "CO-9"}},
				Warnings: []migration.SkippedRow{{ExternalID: "P-2", Line: 3, Reason: "link dropped", Kind: migration.ProblemDroppedLink}},
			},
		}},
	}
}

func TestAZipRunNamesItsPartsAndNoSingleObject(t *testing.T) {
	out := toContractImportRun(zipRun())
	if out.Object != nil || out.FileName == nil || *out.FileName != "export.zip" || out.SourceName == nil || *out.SourceName != "export" {
		t.Fatalf("run = %+v, want no object and the file and source names", out)
	}
	if out.Parts == nil || len(*out.Parts) != 2 || (*out.Parts)[1].RowCount != 3 || (*out.Parts)[1].Object != crmcontracts.ImportObjectContact {
		t.Fatalf("parts = %+v, want companies then contacts with 3 rows", out.Parts)
	}
}

func TestAZipReportCountsEachPartAndNamesTheFileOfEveryIssue(t *testing.T) {
	out := toContractImportReport(zipRun())
	if out.Parts == nil || len(*out.Parts) != 2 {
		t.Fatalf("parts = %+v", out.Parts)
	}
	contacts := (*out.Parts)[1].Disposition
	if contacts.Created != 2 || contacts.Skipped != 1 || contacts.Unchanged != 0 {
		t.Errorf("contacts = %+v, want 2 created and 1 skipped — a warning is not a skip", contacts)
	}
	if out.Disposition.Created != 4 || out.Disposition.Skipped != 1 || out.RowsRead != 5 {
		t.Errorf("totals = %+v over %d rows, want the parts added", out.Disposition, out.RowsRead)
	}
	if len(out.Issues) != 2 {
		t.Fatalf("issues = %+v, want the skip and the warning", out.Issues)
	}
	broken := out.Issues[0]
	if broken.File == nil || *broken.File != "contacts.csv" || broken.Kind == nil || *broken.Kind != crmcontracts.ImportProblemKindBrokenLink ||
		broken.Value == nil || *broken.Value != "CO-9" || broken.Column == nil || *broken.Column != "company_ref" {
		t.Errorf("issue = %+v, want file, kind, column and value carried", broken)
	}
}

// Compose holds this test because a module may not import the contract
// package. The contract side is read from crm.yaml, so a kind added on either
// side alone fails here.
func TestEveryProblemKindIsAContractValueAndBack(t *testing.T) {
	schema, ok := loadContract(t).Components.Schemas["ImportProblemKind"]
	if !ok || schema.Value == nil || len(schema.Value.Enum) == 0 {
		t.Fatal("crm.yaml declares no ImportProblemKind enum")
	}
	var contract []string
	for _, v := range schema.Value.Enum {
		kind, isText := v.(string)
		if !isText {
			t.Fatalf("ImportProblemKind value %v is not text", v)
		}
		contract = append(contract, kind)
	}
	got := migration.ProblemKinds()
	slices.Sort(got)
	slices.Sort(contract)
	if !slices.Equal(got, contract) {
		t.Fatalf("migration kinds %v, contract kinds %v", got, contract)
	}
}

// The dry run and the commit each list a row's warning, and the report names it
// once. A warning keyed like a skip of the same row is still named.
func TestAWarningIsNamedOnceAndNeverFoldedOntoASkip(t *testing.T) {
	run := zipRun()
	contacts := &run.Report.Objects[1]
	contacts.Warnings = append(contacts.Warnings, contacts.Warnings[0], migration.SkippedRow{
		ExternalID: "P-3", Line: 4, Reason: "owner unknown", Kind: migration.ProblemUnknownOwner,
	})
	out := toContractImportReport(run)
	if len(out.Issues) != 3 {
		t.Fatalf("issues = %+v, want the skip, the warning once, and the warning on the skipped row", out.Issues)
	}
	if out.Disposition.Skipped != 1 {
		t.Errorf("skipped = %d, want 1: warnings count nowhere", out.Disposition.Skipped)
	}
}

func TestAPartsRequestNamingAFileTwiceOrNoneIsRefused(t *testing.T) {
	lead := crmcontracts.ImportPartRequest{File: "a.csv", Object: crmcontracts.ImportObjectLead, Mapping: map[string]string{"email": "email"}}
	company := crmcontracts.ImportPartRequest{File: "a.csv", Object: crmcontracts.ImportObjectCompany, Mapping: map[string]string{"name": "display_name"}}
	blank := company
	blank.File = " "
	for name, req := range map[string]crmcontracts.CreateImportRunRequest{
		"repeated": partsRequest(lead, company),
		"blank":    partsRequest(lead, blank),
	} {
		_, err := stagedMappingFrom(req)
		var v *httperr.DetailedError
		if !errors.As(err, &v) || v.Fields[0].Code != "file_name" {
			t.Errorf("%s: err = %v, want a file_name refusal", name, err)
		}
	}
}

func TestAPartsRequestWithATopLevelSourceKeyIsRefused(t *testing.T) {
	req := partsRequest(crmcontracts.ImportPartRequest{File: "a.csv", Object: crmcontracts.ImportObjectLead, Mapping: map[string]string{"email": "email"}})
	key := "email"
	req.SourceKey = &key
	_, err := stagedMappingFrom(req)
	var v *httperr.DetailedError
	if !errors.As(err, &v) || v.Fields[0].Code != "one_shape" {
		t.Fatalf("err = %v, want one_shape: a part carries its own source_key", err)
	}
}

func TestARunWithTwoFilesOfOneTypeGetsNoWriters(t *testing.T) {
	twice := migration.RunMapping{Parts: []migration.PartMapping{
		{File: "a.csv", Object: migration.ObjectCompany}, {File: "b.csv", Object: migration.ObjectCompany},
	}}
	if _, err := newBundleWriters(t.Context(), nil, migration.RunID{}, &twice); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("err = %v, want a conflict rather than one file's writer silently replacing the other's", err)
	}
}
