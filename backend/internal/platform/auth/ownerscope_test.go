// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A deal is read whole by every seat, so its read clause renders nothing for a
// rep. The owner scope is the question that clause stopped asking — own/team or
// a live grant — for data that hangs off the deal without being identity.
func TestTheOwnerScopeNarrowsATableEverySeatReadsWhole(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	rep := principal.WithActor(context.Background(), human(principal.RowScopeOwn))

	read, err := ScopeClauseFor(rep, "deal", "d", arg)
	if err != nil {
		t.Fatal(err)
	}
	if read != "" {
		t.Fatalf("the premise moved: a rep's deal read clause is now %q, so the owner scope may no longer be needed", read)
	}

	owned, err := OwnerScopeClauseFor(rep, "deal", "d", arg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(owned, "d.owner_id") {
		t.Errorf("the owner scope does not test the deal's owner: %s", owned)
	}
	if !strings.Contains(owned, "record_grant") {
		t.Errorf("the owner scope dropped the grant arm, so a share would not widen it: %s", owned)
	}

	// An unbounded seat and the system principal narrow nothing.
	for name, p := range map[string]principal.Principal{
		"row_scope=all": human(principal.RowScopeAll),
		"system":        {Type: principal.PrincipalSystem, ID: "system:test"},
	} {
		clause, err := OwnerScopeClauseFor(principal.WithActor(context.Background(), p), "deal", "d", arg)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "" {
			t.Errorf("%s got an owner scope %q, want none", name, clause)
		}
	}

	// And a table name the primitive does not know is an error, never SQL.
	if _, err := OwnerScopeClauseFor(rep, "deal; DROP TABLE deal", "d", arg); err == nil {
		t.Error("an unknown table name was rendered rather than refused")
	}
}
