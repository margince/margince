// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A credential releases only a change that would have gone straight through
// but for a person's earlier edit: never a send, a deal close, a relink, a tag
// merge or a schema change, each of which its own route stages for a human.
func TestOnlyAStraightThroughRecordChangeIsAnUndoableRelease(t *testing.T) {
	for _, tc := range []struct {
		kind, target string
		want         bool
	}{
		{"update_record", "company", true},
		{"update_record", "contact", true},
		{"create_record", "contact", true},
		{"send_email", "activity", false},
		{"send_company_email", "activity", false},
		{"invite_meeting", "contact", false},
		{"enrich", "company", false},
		{"advance_deal", "deal", false},
		{"relink_activity", "activity", false},
		{"relink_activities", "activity", false},
		{"relink_thread", "activity", false},
		{"merge_tags", "tag", false},
		{"create_record", "custom_field", false},
		{"update_record", "custom_field", false},
		{"create_record", "webhook_subscription", false},
		{"update_record", "webhook_subscription", false},
		{"update_record", "", false},
		{"update_record", "a_type_nobody_declared", false},
		{"a_kind_nobody_declared", "company", false},
	} {
		if got := undoableAgentRelease(tc.kind, tc.target); got != tc.want {
			t.Errorf("undoableAgentRelease(%q, %q) = %v, want %v", tc.kind, tc.target, got, tc.want)
		}
	}
}

// Every route the contract itself puts before a human — confirm-first, or
// dynamic so it may — is irreversible for this purpose, read from the
// admission table so a new confirm-first route cannot arrive undoable.
func TestNoConfirmFirstRouteIsAnUndoableRelease(t *testing.T) {
	checked := 0
	for route, pol := range agentPolicies {
		if pol.Tool == "" || pol.Tier == tierAutoExecute {
			continue
		}
		checked++
		if undoableAgentRelease(pol.Tool, string(pol.RecordType)) {
			t.Errorf("%s (%s on %q) is %s and reads as undoable", route, pol.Tool, pol.RecordType, pol.Tier)
		}
	}
	if checked == 0 {
		t.Fatal("no confirm-first operation in the admission table — this compared nothing")
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
