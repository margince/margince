// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
)

func TestCommercialPreviewCoversTheRegisteredStageAndPatchTools(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	covered := map[string]bool{}
	for _, spec := range registry.Specs() {
		if !registry.Stageable(spec.Name) {
			continue
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		var field string
		if _, stage := schema.Properties["to_stage_id"]; stage {
			payload, field = map[string]any{"to_stage_id": "destination"}, "stage_id"
		} else if schema.Properties["fields"] != nil && schema.Properties["id"] != nil {
			payload, field = map[string]any{"fields": map[string]any{"owner_id": "successor"}}, "owner_id"
		} else {
			continue
		}
		changes, supported, err := commercialChanges(crmcontracts.Approval{Kind: spec.Name, ProposedChange: &payload})
		if err != nil || !supported || changes[field] == nil {
			t.Errorf("registered commercial stager %s lacks its %s preview: %v", spec.Name, field, err)
		}
		covered[spec.Name] = true
	}
	for operation, tool := range twinnedOperations(registry) {
		if tool == "advance_deal" || tool == "progress_deal" || tool == "update_record" {
			if !covered[tool] {
				t.Errorf("REST operation %s has no commercial preview for its stager %s", operation, tool)
			}
		}
	}
	if len(covered) == 0 {
		t.Fatal("the registry census found no commercial changes")
	}
}

func TestCommercialPreviewAccountsForEveryGovernedKind(t *testing.T) {
	covered := map[string]string{
		"merge_tags":            `{}`,
		"archive_record":        `{}`,
		"emit_flow_event":       `{ "question":"Ready?" }`,
		"create_record":         `{}`,
		"relink_activity":       `{}`,
		"relink_activities":     `{}`,
		"relink_thread":         `{}`,
		"promote_lead":          `{"trigger":"human_qualify"}`,
		"demote_lead":           `{"reason":"Wrong prospect"}`,
		"send_message":          `{}`,
		"invite_meeting":        `{}`,
		"book_meeting":          `{}`,
		"enrich":                `{}`,
		"disqualify_lead":       `{}`,
		"send_email":            `{"to":["buyer@example.test"],"body":"Delivery Friday"}`,
		"send_company_email":    `{"to":["buyer@example.test"],"body":"Delivery Friday"}`,
		"advance_project_phase": `{"to_phase":"closed","reason":"Delivered"}`,
		"stage_progression":     `{"to_stage_id":"stage"}`,
		"advance_deal":          `{"to_stage_id":"stage"}`,
		"progress_deal":         `{"to_stage_id":"stage"}`,
		"update_record":         `{"fields":{"owner_id":null}}`,
		"assign_owner":          `{"owner_id":null}`,
		"lifecycle_change":      `{"proposed_lifecycle":"former_customer"}`,
		"set_up_partner":        `{"partner_role":"hosting"}`,
	}
	seen := map[string]bool{}
	for _, kind := range approvals.StageableKinds() {
		raw, expected := covered[kind]
		payload := map[string]any{}
		if expected {
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				t.Fatal(err)
			}
		}
		changes, supported, err := commercialChanges(crmcontracts.Approval{Kind: kind, ProposedChange: &payload})
		if err != nil || supported != expected {
			t.Errorf("governed kind %s: covered=%v, want %v; %v", kind, supported, expected, err)
		}
		if expected && kind != "merge_tags" && kind != operationSetUpPartner && kind != "archive_record" && kind != "emit_flow_event" && kind != "create_record" && kind != "relink_activity" && kind != "relink_activities" && kind != "relink_thread" && kind != "promote_lead" && kind != "demote_lead" && kind != "send_message" && kind != "invite_meeting" && kind != "book_meeting" && kind != "enrich" && kind != previewSendEmail && kind != previewSendCompanyEmail && len(changes) == 0 {
			t.Errorf("governed kind %s has no commercial effect", kind)
		}
		seen[kind] = true
	}
	for kind := range covered {
		if !seen[kind] {
			t.Errorf("preview kind %s is not governed", kind)
		}
	}
}

func TestBulkOwnerChangesUseTheirOwnConversationConfirmation(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	for _, spec := range registry.Specs() {
		if spec.Name != "bulk_update_records" {
			continue
		}
		if registry.Stageable(spec.Name) || !spec.ConfirmsInConversation {
			t.Fatal("bulk changes moved into the approval queue without preview coverage")
		}
		return
	}
	t.Fatal("the bulk change producer disappeared from the registry")
}
