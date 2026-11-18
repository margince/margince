// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Row scope never widens what the reader works on: an admin's clause asks the
// same owner, team and grant questions a rep's does.
func TestWorksOnAsksOwnerTeamAndGrantWhateverTheRowScope(t *testing.T) {
	for _, scope := range []principal.RowScope{principal.RowScopeOwn, principal.RowScopeAll} {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		ctx := principal.WithActor(context.Background(), human(scope))
		clause, err := WorksOnClause(ctx, "deal", "d", arg)
		if err != nil {
			t.Fatal(err)
		}
		for _, arm := range []string{"d.owner_id IN", "team_membership", "record_grant"} {
			if !strings.Contains(clause, arm) {
				t.Errorf("%s: works-on clause lacks %q: %s", scope, arm, clause)
			}
		}
		if len(args) != 2 {
			t.Errorf("%s: the clause bound %d arguments, want the member and their teams", scope, len(args))
		}
	}
	if _, err := WorksOnClause(principal.WithActor(context.Background(), human(principal.RowScopeAll)), "pipeline", "p", func(any) int { return 1 }); err == nil {
		t.Error("a table no grant can share was rendered rather than refused")
	}
}

// A message is the reader's when they captured or are assigned it, or when a
// record it is linked to is theirs.
func TestActivityWorksOnWalksEveryLinkedRecordType(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	clause, err := ActivityWorksOnClause(principal.WithActor(context.Background(), human(principal.RowScopeTeam)), "e", arg)
	if err != nil {
		t.Fatal(err)
	}
	for _, arm := range []string{"e.captured_by", "e.assignee_id", "activity_link", "JOIN deal", "JOIN contact", "JOIN company", "JOIN lead", "JOIN project"} {
		if !strings.Contains(clause, arm) {
			t.Errorf("activity works-on clause lacks %q: %s", arm, clause)
		}
	}
}

// An actor with no member behind it works on nothing, so it is shown nothing.
func TestAnActorWithNoMemberWorksOnNothing(t *testing.T) {
	system := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	arg := func(any) int { return 1 }
	records, err := WorksOnClause(system, "contact", "c", arg)
	if err != nil || records != "FALSE" {
		t.Errorf("records: %q, %v; want FALSE", records, err)
	}
	activities, err := ActivityWorksOnClause(system, "e", arg)
	if err != nil || activities != "FALSE" {
		t.Errorf("activities: %q, %v; want FALSE", activities, err)
	}
}
