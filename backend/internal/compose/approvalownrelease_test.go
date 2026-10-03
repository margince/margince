// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A message sent, a page fetched or a webhook registered cannot be recalled, so
// a credential never releases one it proposed; a record change it may.
func TestOnlyARecordChangeIsAnUndoableRelease(t *testing.T) {
	for _, tc := range []struct {
		kind, target string
		want         bool
	}{
		{"update_record", "company", true},
		{"create_record", "contact", true},
		{"send_email", "activity", false},
		{"send_company_email", "activity", false},
		{"invite_meeting", "contact", false},
		{"enrich", "company", false},
		{"create_record", "webhook_subscription", false},
		{"update_record", "webhook_subscription", false},
		{"a_kind_nobody_declared", "company", false},
	} {
		if got := undoableAgentRelease(tc.kind, tc.target); got != tc.want {
			t.Errorf("undoableAgentRelease(%q, %q) = %v, want %v", tc.kind, tc.target, got, tc.want)
		}
	}
}

// Every verb whose operation spends an egressing cap is irreversible, read
// from the admission table itself so a new send verb cannot arrive undoable.
func TestNoEgressingVerbIsAnUndoableRelease(t *testing.T) {
	checked := 0
	for route, pol := range agentPolicies {
		if pol.Tool == "" || !principal.Scope(pol.Scope).Egresses() {
			continue
		}
		checked++
		if undoableAgentRelease(pol.Tool, string(pol.RecordType)) {
			t.Errorf("%s (%s) spends the %s cap and reads as undoable", route, pol.Tool, pol.Scope)
		}
	}
	if checked == 0 {
		t.Fatal("no egressing operation in the admission table — this compared nothing")
	}
}
