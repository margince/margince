// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The brief's entry asks the activity and contact grants and has never asked
// the deal one, so the deal band's only bound was scopeFor — which under
// row_scope=all returns `TRUE` and admits every deal in the workspace.
//
// A unit test reaches it because scopeFor is a pure function of the principal:
// the clause it returns is what the statement is built from, and asserting the
// clause is asserting what the database was asked.
func TestScopeForMatchesNoRowWithoutTheObjectGrant(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			// Everything the brief's entry asks for, and no deal grant.
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true},
				"contact":  {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	clause, err := scopeFor(ctx, "deal", "dd", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("scopeFor: %v", err)
	}
	if clause != scopeNone {
		t.Errorf("scopeFor(deal) = %q, want %q — the join must match nothing, or the "+
			"brief names the deal's figure and close date to a seat that may not read deals",
			clause, scopeNone)
	}
}

// The positive control, without which the assertion above is satisfied by a
// scopeFor that refused everything.
func TestScopeForAdmitsTheGrantedObject(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	clause, err := scopeFor(ctx, "deal", "dd", func(any) int { return 1 })
	if err != nil {
		t.Fatalf("scopeFor: %v", err)
	}
	if clause != scopeAll {
		t.Errorf("scopeFor(deal) = %q, want %q for a granted seat at row_scope=all", clause, scopeAll)
	}
}

// The room's deal amount is read back by NAME from a derived table, so the
// masked rendering has to be aliased.
//
// A masked rendering is an expression, and Postgres names an unaliased
// expression with a placeholder — so the outer select resolves nothing and a
// masked caller gets a column-not-found error instead of a withheld figure.
// That is a failure only a masked seat ever sees, which is exactly the kind
// that ships.
//
// A template assertion rather than a database run: what broke was the SQL's
// own text, and this fails the moment somebody drops the alias.
func TestTheRoomStatementAliasesTheMaskedAmount(t *testing.T) {
	t.Parallel()
	if !strings.Contains(meetingQuery, "%[8]s AS amount_minor") {
		t.Error("the deal amount in the room's LATERAL is no longer aliased — a masked caller " +
			"reads a placeholder column name and the statement fails for them alone")
	}
}
