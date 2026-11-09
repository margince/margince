// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openenums

// The generated Go clients keep a response whose enum value they do not know.
// Each test decodes a real generated type, the way a caller reads a response.

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/pkg/extension/crm"
)

func TestTheContractTypesKeepAResponseWithAnUnknownEnumValue(t *testing.T) {
	body := `{
		"lanes_omitted": ["a_lane_from_a_later_release"],
		"needs_you": [{"source": "a_source_from_a_later_release", "actions": ["an_action_from_a_later_release"]}]
	}`
	var got crmcontracts.Attention
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("an unknown enum value failed the whole response: %v", err)
	}
	if got.LanesOmitted == nil || (*got.LanesOmitted)[0] != "a_lane_from_a_later_release" {
		t.Errorf("the unknown lane was not kept: %+v", got.LanesOmitted)
	}
	if len(got.NeedsYou) != 1 || got.NeedsYou[0].Source != "a_source_from_a_later_release" ||
		got.NeedsYou[0].Actions[0] != "an_action_from_a_later_release" {
		t.Errorf("the unknown item values were not kept: %+v", got.NeedsYou)
	}
	if got.NeedsYou[0].Source.Valid() {
		t.Error("an unknown value reads as valid, so a caller cannot tell it apart")
	}
}

func TestTheExtensionTypesKeepAResponseWithAnUnknownEnumValue(t *testing.T) {
	var got crm.Deal
	if err := json.Unmarshal([]byte(`{"status": "a_status_from_a_later_release", "priority": "urgent_later"}`), &got); err != nil {
		t.Fatalf("an unknown enum value failed the whole record: %v", err)
	}
	if got.Status != "a_status_from_a_later_release" || got.Priority == nil || *got.Priority != "urgent_later" {
		t.Errorf("the unknown values were not kept: status %q priority %v", got.Status, got.Priority)
	}
}
