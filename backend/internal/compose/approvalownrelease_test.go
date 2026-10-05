// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
		if got := undoableWithoutGuard(tc.kind, tc.target, nil); got != tc.want {
			t.Errorf("undoableWithoutGuard(%q, %q) = %v, want %v", tc.kind, tc.target, got, tc.want)
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
		if undoableWithoutGuard(pol.Tool, string(pol.RecordType), nil) {
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
		if undoableWithoutGuard(pol.Tool, string(pol.RecordType), nil) {
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
		if undoableWithoutGuard(pair.tool, pair.recordType, nil) {
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

// A thread's approval binds a key, not rows, so it is never undoable whatever
// the destination: a proposal staged before thread moves stopped staging stays
// the contact's to release.
func TestAThreadRelinkIsNeverAnUndoableRelease(t *testing.T) {
	for _, target := range []string{"activity", "company", ""} {
		if undoableWithoutGuard("relink_thread", target, json.RawMessage(`{"entity_type":"company"}`)) {
			t.Errorf("relink_thread onto a company, staged under %q, reads as undoable", target)
		}
	}
}

// A relink is judged by where the staged call files its activities, not by the
// policy's static "dynamic": every link target is something a member can put
// back — a project filing through its undo — and whatever the target type the approval was staged
// under (the batch doors stage under the destination, the single one under the
// activity).
func TestARelinkIsUndoableByItsDestination(t *testing.T) {
	for _, tool := range []string{"relink_activity", "relink_activities"} {
		for _, tc := range []struct {
			change string
			want   bool
		}{
			{`{"entity_type":"company"}`, true},
			{`{"entity_type":"deal"}`, true},
			{`{"entity_type":"contact"}`, true},
			{`{"entity_type":"lead"}`, true},
			// A project is undoable by state, which needs the guard; without one
			// it is the stricter answer. TestAProjectRelinkIsReleasableOnlyWhile
			// ItsFilingsStayUndoable holds the guarded half.
			{`{"entity_type":"project"}`, false},
			{`{"entity_type":"webhook_subscription"}`, false},
			{`{}`, false},
			{`not json`, false},
		} {
			for _, target := range []string{"activity", "company", ""} {
				if got := undoableWithoutGuard(tool, target, json.RawMessage(tc.change)); got != tc.want {
					t.Errorf("undoableWithoutGuard(%q, %q, %s) = %v, want %v", tool, target, tc.change, got, tc.want)
				}
			}
		}
	}
}

// Every dynamic tool on the surface is either judged by its resolved
// destination or named here as one whose staged call stays the human's, so a
// new dynamic tool cannot arrive classified by its static label unnoticed.
func TestEveryDynamicToolIsJudgedByItsCallOrStaysHumanReleased(t *testing.T) {
	// A won or lost deal move turns on the pipeline's semantics rather than a
	// destination; closing a deal is a decision a contact keeps.
	// A thread's approval binds a key that the conversation may outgrow, not rows.
	humanReleased := map[string]bool{"advance_deal": true, "progress_deal": true, "relink_thread": true}
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
		if undoableWithoutGuard(spec.Name, "", nil) {
			t.Errorf("%s reads as undoable with no call to resolve", spec.Name)
		}
	}
	if dynamic == 0 {
		t.Fatal("no dynamic tool is registered — this compared nothing")
	}
}

// undoableWithoutGuard is the classification with no filing guard installed: a project
// destination is then not undoable, the stricter answer.
func undoableWithoutGuard(kind, target string, change json.RawMessage) bool {
	return undoableAgentRelease(nil)(context.Background(), nil, approvals.StagedCall{Kind: kind, TargetType: target, Change: change})
}

type fakeFilingGuard struct {
	stay   bool
	asked  []ids.UUID
	failed bool
}

func (g *fakeFilingGuard) guard() filingGuard {
	return func(_ context.Context, _ activities.Querier, named []ids.UUID, _ ids.UUID) (bool, error) {
		g.asked = append(g.asked, named...)
		if g.failed {
			return false, errors.New("the read failed")
		}
		return g.stay, nil
	}
}

// A project destination is undoable by state: the named activities must still be
// ones the undo could take back, and a guard that cannot answer, or is absent,
// refuses. Every other destination is unaffected by the guard.
func TestAProjectRelinkIsReleasableOnlyWhileItsFilingsStayUndoable(t *testing.T) {
	one := ids.NewV7()
	project := `"entity_type":"project","entity_id":"` + ids.NewV7().String() + `"`
	single := json.RawMessage(`{"activity_id":"` + one.String() + `",` + project + `}`)
	routed := json.RawMessage(`{` + project + `}`)
	set := json.RawMessage(`{"activity_ids":["` + one.String() + `"],` + project + `}`)
	company := json.RawMessage(`{"activity_ids":["` + one.String() + `"],"entity_type":"company"}`)

	for _, tc := range []struct {
		name  string
		guard *fakeFilingGuard
		call  approvals.StagedCall
		want  bool
	}{
		{"a single filing that stays undoable", &fakeFilingGuard{stay: true}, approvals.StagedCall{Kind: "relink_activity", TargetType: "activity", Change: single}, true},
		{"a single filing whose activity is named by the route", &fakeFilingGuard{stay: true}, approvals.StagedCall{Kind: "relink_activity", TargetType: "activity", TargetID: one, Change: routed}, true},
		{"a routed filing the undo could not take back", &fakeFilingGuard{stay: false}, approvals.StagedCall{Kind: "relink_activity", TargetType: "activity", TargetID: one, Change: routed}, false},
		{"a set that stays undoable", &fakeFilingGuard{stay: true}, approvals.StagedCall{Kind: "relink_activities", TargetType: "company", Change: set}, true},
		{"a restricted, held or erasure-pending activity", &fakeFilingGuard{stay: false}, approvals.StagedCall{Kind: "relink_activities", TargetType: "company", Change: set}, false},
		{"a guard that cannot answer", &fakeFilingGuard{failed: true}, approvals.StagedCall{Kind: "relink_activities", TargetType: "company", Change: set}, false},
		{"a company is never asked", &fakeFilingGuard{stay: false}, approvals.StagedCall{Kind: "relink_activities", TargetType: "company", Change: company}, true},
	} {
		got := undoableAgentRelease(tc.guard.guard())(context.Background(), nil, tc.call)
		if got != tc.want {
			t.Errorf("%s: undoable = %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := undoableAgentRelease(nil)(context.Background(), nil, approvals.StagedCall{Kind: "relink_activities", Change: set}); got {
		t.Error("a project relink with no guard installed is undoable, want the stricter answer")
	}
	one2 := &fakeFilingGuard{stay: true}
	undoableAgentRelease(one2.guard())(context.Background(), nil, approvals.StagedCall{Kind: "relink_activity", TargetType: "activity", Change: single})
	if len(one2.asked) != 1 || one2.asked[0] != one {
		t.Errorf("the single form asked about %v, want the one activity it names", one2.asked)
	}
}
