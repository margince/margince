// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package principal

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAgentRunIDRoundTrips(t *testing.T) {
	id := ids.NewV7()
	ctx := WithAgentRunID(context.Background(), id)
	got, ok := AgentRunID(ctx)
	if !ok || got != id {
		t.Fatalf("AgentRunID = %v, %v; want %v, true", got, ok, id)
	}
	if _, ok := AgentRunID(context.Background()); ok {
		t.Fatal("AgentRunID on a bare context should report ok=false")
	}
}

// A preview runs under the caller's policy Within a role's: only what both
// allow, the narrower scope, and every mask either carries.
func TestWithinKeepsOnlyWhatBothPoliciesGrant(t *testing.T) {
	admin := Permissions{
		RowScope: RowScopeAll,
		Objects: map[string]ObjectGrant{
			"deal":    {Read: true, Update: true},
			"company": {Read: true},
		},
		FieldMasks: []FieldMask{{Object: "deal", Field: "margin", Condition: MaskAlways}},
	}
	rep := Permissions{
		RoleKeys: []string{"rep"},
		RowScope: RowScopeOwn,
		Objects: map[string]ObjectGrant{
			"deal":   {Read: true, Create: true},
			"report": {Read: true},
		},
		FieldMasks: []FieldMask{{Object: "deal", Field: "amount_minor", Condition: MaskOutsideWriteAuthority}},
	}
	got := admin.Within(rep)
	if got.RowScope != RowScopeOwn {
		t.Errorf("row scope %q, want the narrower own", got.RowScope)
	}
	if !got.Allows("deal", ActionRead) || got.Allows("deal", ActionUpdate) || got.Allows("deal", ActionCreate) {
		t.Errorf("deal grant %+v, want read alone", got.Objects["deal"])
	}
	if got.Allows("company", ActionRead) || got.Allows("report", ActionRead) {
		t.Error("an object only one policy grants survived")
	}
	if len(got.FieldMasks) != 2 {
		t.Errorf("masks %v, want both policies' masks", got.FieldMasks)
	}
	if got.RowScope.Wider(admin.RowScope) || rep.Within(admin).RowScope != RowScopeOwn {
		t.Error("Within widened a scope")
	}
}
