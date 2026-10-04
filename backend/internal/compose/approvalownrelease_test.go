// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// A credential releases only a change that would have gone straight through
// but for a human's earlier edit: never a send, a deal close, a relink, a tag
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
		if got := undoableAgentRelease(tc.kind, tc.target, nil); got != tc.want {
			t.Errorf("undoableAgentRelease(%q, %q) = %v, want %v", tc.kind, tc.target, got, tc.want)
		}
	}
}

// Every route the contract itself puts before a human — confirm-first, or
// dynamic so it may — is irreversible for this purpose until a call says where
// it resolves, read from the admission table so a new confirm-first route cannot
// arrive undoable.
func TestNoConfirmFirstRouteIsAnUndoableRelease(t *testing.T) {
	checked := 0
	for route, pol := range agentPolicies {
		if pol.Tool == "" || pol.Tier == tierAutoExecute {
			continue
		}
		checked++
		if undoableAgentRelease(pol.Tool, string(pol.RecordType), nil) {
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
		if undoableAgentRelease(pol.Tool, string(pol.RecordType), nil) {
			t.Errorf("%s (%s) spends the %s cap and reads as undoable", route, pol.Tool, pol.Scope)
		}
	}
	if checked == 0 {
		t.Fatal("no egressing operation in the admission table — this compared nothing")
	}
}

// An installation's tier floor stages a call its verb would run straight
// through, so every pair the floor tightens must stay the human's to release —
// or a credential could approve the very confirmation the operator demanded.
func TestNoFlooredPairIsAnUndoableRelease(t *testing.T) {
	if len(contractTierFloors) == 0 {
		t.Fatal("the contract floors no (verb, record type) pair — this compared nothing")
	}
	for pair := range contractTierFloors {
		if undoableAgentRelease(pair.tool, pair.recordType, nil) {
			t.Errorf("%s on %q is floored confirm-first and reads as undoable", pair.tool, pair.recordType)
		}
	}
}

// The classifier reads the admission table, but the tool door stages on the
// registered spec: a verb declared confirm-first or dynamic there stages calls
// the table calls straight-through, and its credential must not release them.
func TestEveryUndoablePairIsAStraightThroughTool(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	for pair, undoable := range agentStraightThrough {
		if !undoable {
			continue
		}
		spec, registered := registry.Spec(pair.tool)
		if !registered || spec.Tier != mcp.TierAutoExecute || spec.TierResolver != nil {
			t.Errorf("%s on %q reads as undoable, but its tool is not a static auto-execute verb", pair.tool, pair.recordType)
		}
	}
}

// A relink is judged by where the staged call files its activities, not by the
// policy's static "dynamic": every destination but a project is an association
// a member relinks back, and whatever the target type the approval was staged
// under (the batch doors stage under the destination, the single one under the
// activity).
func TestARelinkIsUndoableByItsDestination(t *testing.T) {
	for _, tool := range []string{"relink_activity", "relink_activities", "relink_thread"} {
		for _, tc := range []struct {
			change string
			want   bool
		}{
			{`{"entity_type":"company"}`, true},
			{`{"entity_type":"deal"}`, true},
			{`{"entity_type":"contact"}`, true},
			{`{"entity_type":"lead"}`, true},
			{`{"entity_type":"project"}`, false},
			{`{"entity_type":"webhook_subscription"}`, false},
			{`{}`, false},
			{`not json`, false},
		} {
			for _, target := range []string{"activity", "company", ""} {
				if got := undoableAgentRelease(tool, target, json.RawMessage(tc.change)); got != tc.want {
					t.Errorf("undoableAgentRelease(%q, %q, %s) = %v, want %v", tool, target, tc.change, got, tc.want)
				}
			}
		}
	}
}

// Every dynamic tool on the surface is either judged by its resolved
// destination or named here as one whose staged call stays the human's, so a
// new dynamic tool cannot arrive classified by its static label unnoticed.
func TestEveryDynamicToolIsJudgedByItsCallOrStaysHumanReleased(t *testing.T) {
	// A won or lost deal move is the one dynamic tier that turns on the
	// pipeline's semantics rather than a destination; closing a deal is a
	// decision a contact keeps.
	humanReleased := map[string]bool{"advance_deal": true, "progress_deal": true}
	dynamic := 0
	for _, spec := range NewRegistry(nil, SendPath{}).Specs() {
		if spec.Tier != mcp.TierDynamic {
			continue
		}
		dynamic++
		_, decided := agents.ReleaseUndoableByDestination(spec.Name, json.RawMessage(`{"entity_type":"company"}`))
		switch {
		case decided && humanReleased[spec.Name]:
			t.Errorf("%s is judged by destination and also listed as human-released", spec.Name)
		case !decided && !humanReleased[spec.Name]:
			t.Errorf("%s is dynamic, and nothing says whether its staged call is undoable", spec.Name)
		}
		if undoableAgentRelease(spec.Name, "", nil) {
			t.Errorf("%s reads as undoable with no call to resolve", spec.Name)
		}
	}
	if dynamic == 0 {
		t.Fatal("no dynamic tool is registered — this compared nothing")
	}
}
