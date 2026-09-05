// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Who may record a forecast against a scope.
//
// Separate from store.go because it is a different question from how a call is
// written: that file spells the transaction, the supersession and the audit
// row, and this one spells the single rule about WHOSE forecast a caller may
// assert — the rule the object grant never carried.

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requireForecastScope is the gate every entry point that RECORDS a scope runs:
// the object grant, then the shape, then the scope itself.
//
// The object grant says a seat may make forecast calls at all; it says nothing
// about WHOSE, and that was the whole gate. So a seat holding forecast.create
// recorded a commitment against any owner's or team's scope — stored under that
// scope, superseding its standing call, and unremovable, since no seat holds
// forecast.update or forecast.delete. The owning migration always stated the
// rule ("an assertion about a team's forecast, made by whoever is accountable
// for it"); nothing enforced the second half of that sentence.
//
// WRITES ONLY, and the asymmetry is the point rather than an omission. Measuring
// a population and asserting a number for it are different questions with
// different answers: a team manager may legitimately MEASURE a teammate's
// pipeline, and the composition layer decides that once for every analytics
// surface (compose.AnalyticsPopulationClause), using a live-membership read this
// module cannot make — it owns no tables and may not import identity. Recording
// a call is the stricter question, it is asked of a scope taken verbatim from a
// request body that never reaches that resolver, and it is the module's own. A
// second, narrower copy of the read rule here would refuse the very manager the
// resolver had just admitted.
//
// The shape is checked BEFORE the authority so that a scope_kind nobody
// recognises is a malformed request for every caller. Checked after, an
// unbounded seat was told its field was wrong while a bounded one was told the
// thing it named does not exist — one defect reported two ways, decided by the
// reader's privilege.
//
// Held by: TestEveryForecastScopeWriterAsksWhoAnswersForTheScope
// (backend/gates/forecastscopeauthority_test.go) — it fails when an exported
// store method records against a scope-bearing type and does not reach this.
func requireForecastScope(ctx context.Context, scope Scope) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	if err := checkScope(scope); err != nil {
		return err
	}
	return requireScopeAuthority(ctx, scope)
}

// requireScopeAuthority refuses a scope this caller may not RECORD against.
//
// Answered from the principal rather than from a membership query: the auth
// layer has already resolved the caller's LIVE team memberships onto TeamIDs —
// an archived team resolves neither row scope nor a team share — so the module
// keeps the property its package doc rests on, owning no tables and needing no
// database to decide anything.
//
// A miss is ErrNotFound rather than ErrPermissionDenied, which is the answer the
// analytics resolver gives for the same reason: a refusal naming a specific id
// confirms that id exists, which is the disclosure the rule is for.
//
// The WORKSPACE scope is deliberately not narrowed. It names no subject, so
// there is no membership to test and nobody is more accountable for it than
// anybody else holding the grant; what bounds it is the object grant, exactly
// as before this check existed.
//
// The switch is total over what checkScope admits, and checkScope runs first.
// The trailing refusal is the fail-closed default for a kind added to that
// vocabulary and not to this rule — the direction an authorization predicate
// has to fail in.
func requireScopeAuthority(ctx context.Context, scope Scope) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		// No resolved actor is no authority over anyone's scope. Fail closed:
		// a caller the auth layer could not identify is not every caller.
		return apperrors.ErrNotFound
	}
	if auth.Unbounded(p) {
		return nil
	}
	switch scope.Kind {
	case ScopeWorkspace:
		return nil
	case ScopeOwner:
		if scope.ID != nil && *scope.ID == p.UserID && !p.UserID.IsZero() {
			return nil
		}
	case ScopeTeam:
		if scope.ID != nil && slices.Contains(p.TeamIDs, *scope.ID) {
			return nil
		}
	}
	return apperrors.ErrNotFound
}
