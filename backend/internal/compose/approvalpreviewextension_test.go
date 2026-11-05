// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A unit names its own arguments. One named like a detail the client renders as
// markup still reaches the client under the plain-text prefix, and bounded.
func TestAnExtensionArgumentCannotTakeADetailKeyTheClientRendersAsMarkup(t *testing.T) {
	kind := approvals.ExtensionKind{Verb: "forget_record", TargetTable: "ext_demo_record"}
	target, table, summary := openapi_types.UUID(ids.NewV7()), kind.TargetTable, "Forget record (demo)"
	long := strings.Repeat("é", maxArgumentPreviewBytes)
	preview, err := extensionPreview(crmcontracts.Approval{
		Kind: kind.Verb, TargetEntityId: &target, TargetEntityType: &table, Summary: &summary,
		ProposedChange: &map[string]any{"html_body": "<img src=x onerror=alert(1)>", "signature_html": "<b>x</b>", "body": long},
	}, kind, &crmcontracts.ApprovalPreview{})
	if err != nil || preview.State != previewReadyState || preview.Details == nil {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	for _, detail := range *preview.Details {
		if detail.Field != "summary" && !strings.HasPrefix(detail.Field, extensionArgumentPrefix) {
			t.Errorf("argument detail %q is not under %q", detail.Field, extensionArgumentPrefix)
		}
		if len(detail.Value.Text) > maxArgumentPreviewBytes+len("…") {
			t.Errorf("%s carries %d bytes, above the display bound", detail.Field, len(detail.Value.Text))
		}
	}
	body := (*preview.Details)[1]
	if body.Field != extensionArgumentPrefix+"body" || !strings.HasSuffix(body.Value.Text, "é…") {
		t.Errorf("the long argument is shown as %s=%q…, want it cut on a character and marked", body.Field, body.Value.Text[:8])
	}
}
