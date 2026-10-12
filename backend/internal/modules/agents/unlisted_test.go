// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"slices"
	"testing"
)

// advance_deal and relink_thread stay callable by name and are not offered:
// progress_deal takes every argument advance_deal takes and stages the same
// command, and relink_thread refuses every call over MCP in favour of
// relink_activities.
func TestASupersededToolIsNotOffered(t *testing.T) {
	var offered []string
	for _, spec := range fullRegistry(t).Offered(agentCtx()) {
		offered = append(offered, spec.Name)
	}
	for superseded, by := range map[string]string{"advance_deal": "progress_deal", "relink_thread": "relink_activities"} {
		if slices.Contains(offered, superseded) || !slices.Contains(offered, by) {
			t.Errorf("offered %v: want %s in place of %s", offered, by, superseded)
		}
	}
}
