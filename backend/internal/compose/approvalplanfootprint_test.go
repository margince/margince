// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A footprint is stored on the approval for good. It holds the references and
// field names the checks read, and none of the values the plan showed.
func TestAPlanFootprintHoldsReferencesAndFieldNamesButNoValue(t *testing.T) {
	company, contact := ids.NewV7(), ids.NewV7()
	companyType, contactType := entityCompany, string(recordTypeContact)
	details := []crmcontracts.ApprovalPreviewDetail{
		{Field: "to", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueText, Text: "buyer@secret.test"}},
		{Field: "read_profile_field", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueText, Text: "title"}},
		{Field: "read_profile_value", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueText, Text: "Chief Secret Officer"}},
	}
	rows := []crmcontracts.ApprovalPreviewImportRow{{Object: "lead", Changes: []crmcontracts.ApprovalPreviewChange{
		{Field: "phone", After: crmcontracts.ApprovalPreviewValue{Kind: previewValueText, Text: "+49 30 999"}},
	}}}
	preview := &crmcontracts.ApprovalPreview{
		Version: 1, State: previewReadyState, Details: &details, ImportRows: &rows,
		Changes: []crmcontracts.ApprovalPreviewChange{{
			Field:  "company_id",
			Before: crmcontracts.ApprovalPreviewValue{Kind: previewValueText, Text: "Old Secret Ltd"},
			After:  crmcontracts.ApprovalPreviewValue{Kind: previewValueText, Text: "New Secret Ltd", EntityType: &companyType, EntityId: new(openapi_types.UUID(company))},
		}},
	}
	approval := crmcontracts.Approval{Kind: toolUpdateRecord, TargetEntityType: &contactType, TargetEntityId: new(openapi_types.UUID(contact))}

	footprint := planFootprint(approval, preview, nil, true)
	raw, err := json.Marshal(footprint)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"buyer@secret.test", "Chief Secret Officer", "+49 30 999", "Old Secret Ltd", "New Secret Ltd"} {
		if strings.Contains(string(raw), value) {
			t.Errorf("the footprint keeps the value %q: %s", value, raw)
		}
	}
	for _, field := range []string{"to", "title", "phone", "company_id"} {
		if !slices.Contains(footprint.Fields, field) {
			t.Errorf("the footprint does not name the field %q: %s", field, raw)
		}
	}
	if !slices.Contains(footprint.Refs[entityCompany], company) || !slices.Contains(footprint.Refs[contactType], contact) {
		t.Errorf("the footprint lost a record the plan touched: %s", raw)
	}
	if !slices.Contains(footprint.Objects, "lead") {
		t.Errorf("the footprint does not carry the import row's object: %s", raw)
	}
}

// A plan its build withholds from every reader leaves nothing a later reader
// could clear.
func TestAnAgentsUnreadyPlanLeavesNoFootprint(t *testing.T) {
	stale := &crmcontracts.ApprovalPreview{Version: 1, State: previewStaleState}
	if footprint := planFootprint(crmcontracts.Approval{Kind: toolUpdateRecord}, stale, nil, true); footprint != nil {
		t.Fatalf("an agent's stale plan left a footprint: %+v", footprint)
	}
	if footprint := planFootprint(crmcontracts.Approval{Kind: toolUpdateRecord}, stale, nil, false); footprint == nil {
		t.Fatal("a decider's own stale plan, which its pending card showed, left no footprint")
	}
}
