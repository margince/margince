// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A commit starts only from awaiting_approval, which is how the mandatory dry
// run is enforced: a run reaches that state ONLY by producing a report, so
// requiring the state is requiring the report. There is no verb that imports
// without one.
func TestACommitRefusesARunThatHasNotBeenDryRun(t *testing.T) {
	for _, status := range []string{"validating", "running", "complete", "failed", "undoing"} {
		t.Run(status, func(t *testing.T) {
			err := refuseUncommittableRun(crmcontracts.ImportRun{
				Id:     openapi_types.UUID(ids.NewV7()),
				Status: crmcontracts.ImportRunStatus(status),
			})
			if err == nil {
				t.Fatalf("a %s run was accepted for commit; only awaiting_approval may commit", status)
			}
			if !strings.Contains(err.Error(), status) {
				t.Errorf("the refusal does not say which state the run is in: %v", err)
			}
		})
	}
	if err := refuseUncommittableRun(crmcontracts.ImportRun{
		Id: openapi_types.UUID(ids.NewV7()), Status: awaitingApproval,
	}); err != nil {
		t.Errorf("a dry-run run was refused: %v", err)
	}
}

// A column nothing reads is reported, not dropped in silence.
//
// A caller who mistyped a field name would otherwise get a clean report and a
// column quietly missing from every imported row — which looks exactly like a
// successful import until somebody goes looking for the data.
func TestUnmappedColumnsAreNamedRatherThanDroppedSilently(t *testing.T) {
	profile := crmcontracts.ImportSourceProfile{Columns: []crmcontracts.ImportColumn{
		{Header: "Company"}, {Header: "Website"}, {Header: "Internal Ref"},
	}}
	got := unmappedColumns(profile, map[string]string{
		"Company": "display_name",
		"Website": "", // mapped to nothing, which is the same as unmapped
	})
	want := map[string]bool{"Website": true, "Internal Ref": true}
	if len(got) != len(want) {
		t.Fatalf("unmapped = %v, want the two columns nothing reads", got)
	}
	for _, column := range got {
		if !want[column] {
			t.Errorf("%q was reported unmapped and it is mapped", column)
		}
	}
}

// `object` takes organization, person or lead. Not deal, and not activity.
//
// The refused set is what has no import WRITER: offering it would advertise a
// door that answers an error. The accepted set has to match the REST contract's
// own enum, which is what TestEveryToolEnumMatchesTheContractItMirrors holds it
// to.
func TestOnlyTheThreeImportableObjectsAreAccepted(t *testing.T) {
	for _, object := range []string{"deal", "activity", ""} {
		if err := refuseUnimportableObject(object); err == nil {
			t.Errorf("`object` accepted %q; nothing imports it", object)
		}
	}
	for _, object := range []string{importObjectLead, importObjectOrganization, importObjectPerson} {
		if err := refuseUnimportableObject(object); err != nil {
			t.Errorf("`object` refused %q: %v", object, err)
		}
	}
}

// An empty file is refused before anything is stored.
func TestAnEmptyFileIsRefusedRatherThanStored(t *testing.T) {
	var stored bool
	_, err := previewImport{imports: recordingImports{stored: &stored}}.Handle(
		context.Background(), json.RawMessage(`{"object":"lead","csv":"   \n  "}`))
	if err == nil {
		t.Fatal("an empty file was accepted")
	}
	if stored {
		t.Error("an empty file reached the object store")
	}
}

// The caller's mapping wins over the proposal, column by column, and the
// proposal fills the rest — so a caller correcting one guess does not have to
// restate the ones that were right.
func TestACallersMappingOverridesTheProposalColumnByColumn(t *testing.T) {
	out, err := previewImport{imports: recordingImports{
		suggested: map[string]string{"Company": "legal_name", "Website": "domains"},
	}}.Handle(context.Background(), json.RawMessage(
		`{"object":"organization","csv":"Company,Website\nAcme,acme.test\n",`+
			`"mapping":{"Company":"display_name"}}`))
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	var got ImportPreviewResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.Mapping["Company"] != "display_name" {
		t.Errorf("the caller's mapping lost to the proposal: Company = %q", got.Mapping["Company"])
	}
	if got.Mapping["Website"] != "domains" {
		t.Errorf("the proposal did not fill an unstated column: Website = %q", got.Mapping["Website"])
	}
}

type recordingImports struct {
	stubImports
	stored    *bool
	suggested map[string]string
	columns   []string
	targets   []string
}

func (r recordingImports) ProfileSource(
	_ context.Context, object, _ string,
) (crmcontracts.ImportSourceProfile, error) {
	if r.stored != nil {
		*r.stored = true
	}
	profile := crmcontracts.ImportSourceProfile{
		Object:           crmcontracts.ImportObject(object),
		SuggestedMapping: r.suggested,
		Targets:          r.targets,
	}
	for _, header := range r.columns {
		profile.Columns = append(profile.Columns, crmcontracts.ImportColumn{Header: header})
	}
	return profile, nil
}

// The approval a person sees says what the import will DO.
//
// The inbox row is its summary — nothing renders the report beside it — so a
// summary naming only the run id asks somebody to authorise a bulk write to
// their estate without telling them what it does. They would be clicking yes
// on a number they never saw.
func TestTheApprovalSaysWhatTheImportWillDo(t *testing.T) {
	got := describeImport("organization", crmcontracts.ImportRunReport{
		RowsRead: 453,
		Disposition: crmcontracts.ImportRunDisposition{
			Created: 412, Updated: 38, Unchanged: 3,
		},
		Issues: []crmcontracts.ImportRowIssue{{}, {}},
	})
	for _, want := range []string{"453", "412", "38", "organization", "2 row(s) could not be used"} {
		if !strings.Contains(got, want) {
			t.Errorf("the summary %q does not carry %q", got, want)
		}
	}
}

// The unusable count is never quietly dropped: it is the least flattering
// number in the report and the one a person most needs before saying yes.
func TestACleanImportSaysNothingAboutUnusableRows(t *testing.T) {
	got := describeImport("lead", crmcontracts.ImportRunReport{
		RowsRead:    12,
		Disposition: crmcontracts.ImportRunDisposition{Created: 12},
	})
	if strings.Contains(got, "could not be used") {
		t.Errorf("a clean import reported unusable rows: %q", got)
	}
	if !strings.Contains(got, "create 12") {
		t.Errorf("the summary %q does not say what it creates", got)
	}
}

// A run whose report cannot be read stages NO approval. Refusing is better
// than staging one whose summary cannot say what the import does — that is the
// blind yes this whole path exists to prevent.
func TestARunWhoseReportCannotBeReadStagesNoApproval(t *testing.T) {
	_, err := importResolver{imports: reportlessImports{}}.Subject(
		context.Background(), ImportCommand{Verb: ImportVerbCommit, RunID: ids.NewV7()})
	if err == nil {
		t.Fatal("an approval was staged for a run whose report could not be read")
	}
}

type reportlessImports struct{ stubImports }

func (reportlessImports) ReadReport(context.Context, ids.UUID) (crmcontracts.ImportRunReport, error) {
	return crmcontracts.ImportRunReport{}, errors.New("the report is gone")
}

// TestAProposalThatPlacesOnlySomeColumnsIsRefused is the shape a partial
// proposal fails in.
//
// The proposal matches names and nothing more, so a file spelling its columns
// the way a human would — "Company", "City", "Band" — reaches only `id`. A
// screen draws the rest as blanks somebody fills; a tool caller sees a mapping
// that looks like an answer, validates clean, and commits an update with no
// changed fields. That is an import reporting success and writing nothing.
func TestAProposalThatPlacesOnlySomeColumnsIsRefused(t *testing.T) {
	_, err := previewImport{imports: recordingImports{
		suggested: map[string]string{"id": "id"},
		columns:   []string{"id", "Company", "City", "Country", "Band"},
		targets:   []string{"display_name", "address.city", "address.country", "size_band", "id"},
	}}.Handle(context.Background(), json.RawMessage(
		`{"object":"organization","csv":"id,Company,City,Country,Band\nx,Acme,Essen,DE,201-500\n"}`))
	if err == nil {
		t.Fatal("a proposal that placed one column of five was accepted")
	}
	message := err.Error()
	for _, want := range []string{`"Company"`, `"City"`, `"Country"`, `"Band"`, "display_name", "size_band"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal %q does not name %s — a caller cannot fix what it is not told", message, want)
		}
	}
}

// A proposal that places every column is the one case where accepting the
// proposal whole is honest, and it still goes through.
func TestAProposalThatPlacesEveryColumnIsAccepted(t *testing.T) {
	_, err := previewImport{imports: recordingImports{
		suggested: map[string]string{"display_name": "display_name", "size_band": "size_band"},
		columns:   []string{"display_name", "size_band"},
		targets:   []string{"display_name", "size_band"},
	}}.Handle(context.Background(), json.RawMessage(
		`{"object":"organization","csv":"display_name,size_band\nAcme,201-500\n"}`))
	if err != nil {
		t.Fatalf("a complete proposal was refused: %v", err)
	}
}

// A caller who named even one column has made a choice about the rest, so the
// refusal does not fire — the result's `unmapped` list is what reports it.
func TestACallersOwnMappingIsNotSecondGuessed(t *testing.T) {
	_, err := previewImport{imports: recordingImports{
		columns: []string{"Company", "City"},
		targets: []string{"display_name", "address.city"},
	}}.Handle(context.Background(), json.RawMessage(
		`{"object":"organization","csv":"Company,City\nAcme,Essen\n","mapping":{"Company":"display_name"}}`))
	if err != nil {
		t.Fatalf("a caller's own partial mapping was refused: %v", err)
	}
}
