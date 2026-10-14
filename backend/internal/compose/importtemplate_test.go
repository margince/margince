// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"archive/zip"
	"bytes"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"io"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/importguide"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

func TestTheTemplateHeaderShowsALabelAndTheReadmeListsTheOptions(t *testing.T) {
	f := crmcontracts.ImportFormat{
		Version: crmcontracts.ImportFormatVersionMarginceImport1, SizeLimitMb: 10,
		Order: []crmcontracts.ImportObject{crmcontracts.ImportObjectCompany},
		Files: []crmcontracts.ImportFormatFile{
			{File: "companies.csv", Object: crmcontracts.ImportObjectCompany, Columns: []crmcontracts.ImportFormatColumn{
				{Name: "ref", Description: "Your key", Example: "CO-1"},
				{Name: "cf_tier", Label: new("Tier"), Custom: true, Type: new("picklist"), Options: &[]string{"A", "B"}, Description: "x", Example: "A"},
				{Name: "cf_note", Label: new("Note, internal"), Custom: true, Description: "y", Example: "z"},
			}},
		},
	}
	var buf bytes.Buffer
	if err := writeImportTemplate(&buf, f, langcopy.For("en"), dealStages{}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	header := readEntry(t, zr, "companies.csv")
	readme := readEntry(t, zr, "README.txt")
	if header != "ref,Tier,\"Note, internal\"\n" {
		t.Errorf("header %q, want the labels as CSV cells", header)
	}
	for _, want := range []string{"Tier: A, B", "10 MB", "Your key", "CO-1", "companies.csv"} {
		if !strings.Contains(readme, want) {
			t.Errorf("readme lacks %q:\n%s", want, readme)
		}
	}
}

func TestTheTemplateReadmeSpeaksTheLanguageItIsGiven(t *testing.T) {
	var buf bytes.Buffer
	if err := writeImportTemplate(&buf, crmcontracts.ImportFormat{SizeLimitMb: 10}, langcopy.For("de"), dealStages{}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if readme := readEntry(t, zr, "README.txt"); !strings.Contains(readme, importguide.Template.Formulas.De) {
		t.Fatalf("readme is not German:\n%s", readme)
	}
}

func readEntry(t *testing.T, zr *zip.Reader, name string) string {
	t.Helper()
	entry, err := zr.Open(name)
	if err != nil {
		t.Fatalf("opening %s: %v", name, err)
	}
	defer func() {
		if err := entry.Close(); err != nil {
			t.Errorf("closing %s: %v", name, err)
		}
	}()
	body, err := io.ReadAll(entry)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(body)
}

func TestTheTemplateListsEveryPipelineWithItsStages(t *testing.T) {
	pipelines := []crmcontracts.Pipeline{
		{Id: openapi_types.UUID(ids.NewV7()), Name: "Sales", IsDefault: true, Stages: &[]crmcontracts.Stage{{Id: openapi_types.UUID(ids.NewV7()), Name: "Qualified"}, {Id: openapi_types.UUID(ids.NewV7()), Name: "Won", Semantic: crmcontracts.StageSemanticWon}}},
		{Id: openapi_types.UUID(ids.NewV7()), Name: "Renewals", Stages: &[]crmcontracts.Stage{{Id: openapi_types.UUID(ids.NewV7()), Name: "Due"}}},
	}
	stages := dealStagesFrom(pipelines, importToday)
	readme := templateReadme(crmcontracts.ImportFormat{SizeLimitMb: 10}, langcopy.For("en"), stages)
	for _, want := range []string{"Sales: Qualified, Won", "Renewals: Due"} {
		if !strings.Contains(readme, want) {
			t.Errorf("readme lacks %q:\n%s", want, readme)
		}
	}
	unreadable := templateReadme(crmcontracts.ImportFormat{SizeLimitMb: 10}, langcopy.For("en"), dealStages{})
	if strings.Contains(unreadable, "Sales") || strings.Contains(unreadable, "Pipelines and stages") {
		t.Errorf("a reader who may not read pipelines is shown them:\n%s", unreadable)
	}
}
