// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// A call is an assertion about a scope, made by whoever is accountable for it.
// The object grant says a seat may make calls at all and nothing about WHOSE,
// so it was the whole gate and a manager could record a commitment against a
// rival team — attributed to that team, superseding its standing call, and
// unremovable, since no seat holds forecast.update or forecast.delete.
//
// This is the RECORDING rule and not a reading one. Which population a caller
// may MEASURE is a different question with a wider answer — a team manager may
// read a teammate's figures — and the composition layer settles that once for
// every analytics surface, with a live-membership query this module cannot
// make. Asserting a number for a scope is the stricter question and the only
// one asked here.
//
// Every case here asserts the EXACT refusal, not merely "not allowed". The
// refusal has to be ErrNotFound: whether a rival team has called this period is
// itself the disclosure, so ErrPermissionDenied would leak the existence the
// gate withholds, and a shape error would say the scope was merely malformed.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAScopeIsAnsweredForOnlyByTheSeatAccountableForIt(t *testing.T) {
	me, someoneElse := ids.NewV7(), ids.NewV7()
	myTeam, theirTeam := ids.NewV7(), ids.NewV7()

	seat := func(scope principal.RowScope, teams ...ids.UUID) principal.Principal {
		return principal.Principal{
			Type: principal.PrincipalHuman, UserID: me, TeamIDs: teams,
			Permissions: principal.Permissions{RowScope: scope},
		}
	}
	owner := func(id ids.UUID) Scope { return Scope{Kind: ScopeOwner, ID: &id} }
	team := func(id ids.UUID) Scope { return Scope{Kind: ScopeTeam, ID: &id} }

	cases := []struct {
		name  string
		actor principal.Principal
		scope Scope
		allow bool
	}{
		{"my own owner scope", seat(principal.RowScopeOwn), owner(me), true},
		{"a colleague's owner scope", seat(principal.RowScopeOwn), owner(someoneElse), false},
		{"a colleague's owner scope, from a team seat", seat(principal.RowScopeTeam, myTeam), owner(someoneElse), false},
		{"a team I am on", seat(principal.RowScopeTeam, myTeam), team(myTeam), true},
		{"a team I am not on", seat(principal.RowScopeTeam, myTeam), team(theirTeam), false},
		{"a team, from a seat on no team at all", seat(principal.RowScopeOwn), team(myTeam), false},
		// The workspace scope names no subject, so there is no membership to
		// test and the object grant is what bounds it — unchanged, and asserted
		// so a later narrowing of it is a deliberate edit rather than a surprise.
		{"the workspace scope", seat(principal.RowScopeOwn), Scope{Kind: ScopeWorkspace}, true},
		// An all-scope seat answers for the installation by definition, and the
		// system principal is how the snapshot pass takes every scope's call.
		{"a rival team, held by an all-scope seat", seat(principal.RowScopeAll), team(theirTeam), true},
		{"a colleague's owner scope, held by the system", principal.Principal{Type: principal.PrincipalSystem}, owner(someoneElse), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := requireScopeAuthority(principal.WithActor(context.Background(), c.actor), c.scope)
			switch {
			case c.allow && err != nil:
				t.Errorf("the seat accountable for this scope was refused: %v", err)
			case !c.allow && !errors.Is(err, apperrors.ErrNotFound):
				t.Errorf("a scope this seat does not answer for was allowed or refused the wrong way: %v — "+
					"the refusal must be ErrNotFound, so the answer reads the same as a scope that was never called",
					err)
			}
		})
	}
}

// A context the auth layer never resolved an actor onto is not every caller.
func TestAnUnidentifiedCallerAnswersForNoScope(t *testing.T) {
	id := ids.NewV7()
	for _, scope := range []Scope{{Kind: ScopeWorkspace}, {Kind: ScopeOwner, ID: &id}, {Kind: ScopeTeam, ID: &id}} {
		if err := requireScopeAuthority(context.Background(), scope); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("scope %q with no actor on the context: %v, want ErrNotFound", scope.Kind, err)
		}
	}
}

// requireForecastScope runs checkScope before this, so a named scope carrying no
// id is refused as a malformed request long before it arrives. The predicate is
// still asserted directly on that shape: it is exported to nobody and guarded by
// one caller today, and "some caller validated first" is the assumption a nil
// dereference is made of.
func TestANamedScopeWithNoSubjectIsRefusedRatherThanDereferenced(t *testing.T) {
	actor := principal.Principal{
		Type: principal.PrincipalHuman, UserID: ids.NewV7(),
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn},
	}
	ctx := principal.WithActor(context.Background(), actor)
	for _, kind := range []string{ScopeOwner, ScopeTeam} {
		if err := requireScopeAuthority(ctx, Scope{Kind: kind}); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("scope %q with no id: %v, want ErrNotFound", kind, err)
		}
	}
}
