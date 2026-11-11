// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestConfigurationStateDoesNotConfuseSetupWithRunOutcomes(t *testing.T) {
	owner := ids.New[ids.UserKind]()
	cases := []struct {
		name          string
		record        Automation
		active        bool
		state, reason string
	}{
		{"incomplete stage selection", Automation{Key: stageChangeNotifyName, Params: json.RawMessage(`{}`)}, true, "needs_setup", "configuration_incomplete"},
		{"draft has no authorizer", Automation{Key: postMeetingRecapName, Params: json.RawMessage(`{}`)}, true, "needs_setup", "authorizer_missing"},
		{"authorizer deactivated", Automation{Key: routeLeadName, OwnerID: &owner, Enabled: true, Params: json.RawMessage(`{}`)}, false, "needs_attention", "authorizer_inactive"},
		{"healthy rule with a failed run", Automation{Key: routeLeadName, OwnerID: &owner, Enabled: true, Params: json.RawMessage(`{}`), Runs: &RunSummary{LastRunStatus: new("failed")}}, true, "on", ""},
		{"paused by a colleague", Automation{Key: routeLeadName, Params: json.RawMessage(`{}`)}, true, "paused", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := configurationOf(tc.record, tc.active)
			if got.State != tc.state || got.Reason != tc.reason {
				t.Fatalf("state=%+v, want %s/%s", got, tc.state, tc.reason)
			}
		})
	}
}
