// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestPreviewReadingUsesTheCardLabelsAndValues(t *testing.T) {
	preview := crmcontracts.ApprovalPreview{Version: 2, State: "ready", Details: new([]crmcontracts.ApprovalPreviewDetail{
		{Field: "promotion_contact_name", Value: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: "Demo Contact"}},
		{Field: "promotion_trigger", Value: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: "human_qualify"}},
	})}
	for _, lang := range textlang.Shipped {
		got := ReadPreview(lang, &preview)
		if got.Details[0].Label != PreviewFieldLabel(lang, "promotion_contact_name") || got.Details[0].Label == "Promotion contact name" {
			t.Fatalf("%s: detail label %q", lang, got.Details[0].Label)
		}
		if got.Details[1].Value.Text == "human_qualify" {
			t.Fatalf("%s: coded trigger was not translated", lang)
		}
	}
}

func TestPreviewReadingCarriesAllAuthorizedSections(t *testing.T) {
	preview := crmcontracts.ApprovalPreview{
		Version: 2, State: "ready",
		Conflicts:  new([]crmcontracts.ApprovalPreviewConflict{{Kind: "pending", Summary: "Another proposal remains separate"}}),
		Files:      new([]crmcontracts.ApprovalPreviewFile{{Filename: "Offer.pdf", ByteSize: 120, ContentType: "application/pdf"}}),
		ImportRows: new([]crmcontracts.ApprovalPreviewImportRow{{Line: 2, Object: "company", Outcome: "create", SourceKey: "company-1", Details: []crmcontracts.ApprovalPreviewDetail{{Field: "name", Value: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: "Demo Company"}}}}}),
	}
	got := ReadPreview(textlang.English, &preview)
	if len(got.Conflicts) != 1 || len(got.Files) != 1 || len(got.ImportRows) != 1 || len(got.ImportRows[0].Details) != 1 {
		t.Fatalf("sections = %+v", got)
	}
	preview.State = "withheld"
	got = ReadPreview(textlang.English, &preview)
	if len(got.Conflicts)+len(got.Files)+len(got.ImportRows) != 0 {
		t.Fatal("a withheld preview disclosed sections")
	}
}

func TestPreviewReadingTranslatesLocaleCodesWithRegionNames(t *testing.T) {
	for _, code := range []string{"de-DE", "en-US"} {
		for _, lang := range textlang.Shipped {
			preview := crmcontracts.ApprovalPreview{Version: 2, State: "ready", Details: new([]crmcontracts.ApprovalPreviewDetail{{Field: "locale", Value: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: code}}})}
			got := ReadPreview(lang, &preview)
			if got.Details[0].Value.Text == code {
				t.Fatalf("%s: locale %s was not translated", lang, code)
			}
		}
	}
}
