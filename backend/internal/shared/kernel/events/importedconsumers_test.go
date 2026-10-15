// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import (
	"encoding/json"
	"testing"
)

func TestAnEventIsImportedWhenItsPayloadNamesTheRun(t *testing.T) {
	for name, tc := range map[string]struct {
		eventType string
		payload   string
		want      bool
	}{
		"a deal event naming a run":      {"deal.stage_changed", `{"import_run_id":"0194f1f0-0000-7000-8000-000000000001","imported":true}`, true},
		"a deal event with no run":       {"deal.stage_changed", `{"to_status":"won"}`, false},
		"a deal event with an empty run": {"deal.created", `{"name":"x","import_run_id":""}`, false},
		"no payload at all":              {"deal.archived", ``, false},
		"an unreadable payload":          {"deal.created", `[1,2]`, false},
		"a contact event is never read":  {"contact.created", `{"import_run_id":"0194f1f0-0000-7000-8000-000000000001"}`, false},
	} {
		env := Envelope{Type: tc.eventType}
		if tc.payload != "" {
			env.Payload = json.RawMessage(tc.payload)
		}
		if got := IsImported(env); got != tc.want {
			t.Errorf("%s: IsImported = %v, want %v", name, got, tc.want)
		}
	}
}

func TestEveryDecisionStatesItsPolicyAndReason(t *testing.T) {
	for _, group := range ImportedDecisionGroups() {
		decision, ok := ImportedDecisionOf(group)
		if !ok || decision.Reason == "" || (decision.Policy != SkipImported && decision.Policy != KeepImported) {
			t.Errorf("%s: decision = %+v, want a policy and a reason", group, decision)
		}
	}
	if !SkipsImported(ExtensionGroupPrefix+"demo-react") || SkipsImported("cg:read-model") || SkipsImported("cg:nobody") {
		t.Error("an extension group must skip, a keeper must not, and a group with no entry is not skipped here")
	}
}
