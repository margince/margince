// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The frame an analytics screen builds itself from: which population this
// caller measures by default, which they may choose instead, and what the
// screen may offer them.
//
// One route rather than a field on /me, because the answer depends on the
// installation's own calendar and currency as much as on the caller, and
// because a screen that reads it once has one place to invalidate when a grant
// changes.
//
// The list of allowed scopes is an OFFER, never an authorization. Every data
// route resolves a requested population again through AnalyticsPopulationClause
// — this list only keeps a control from offering something that would be
// refused, which is a different job from refusing it.

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// analyticsContextHandlers serves the frame.
//
// now is injected rather than called inline so a test can state the as-of it
// expects instead of asserting against the wall clock.
type analyticsContextHandlers struct {
	db  *database.DB
	now func() time.Time
}

func newAnalyticsContextHandlers(db *database.DB, now func() time.Time) analyticsContextHandlers {
	return analyticsContextHandlers{db: db, now: now}
}

// GetAnalyticsContext implements GET /analytics/context.
func (h analyticsContextHandlers) GetAnalyticsContext(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var out crmcontracts.AnalyticsContext
	err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		// The same gate the rest of the analytics surface takes: this route
		// discloses which teams and people a caller may measure, which is not
		// a thing to hand somebody who may not read a forecast at all.
		if err := auth.Require(ctx, objectForecast, principal.ActionRead); err != nil {
			return err
		}
		frame, err := readReportFrame(ctx, tx)
		if err != nil {
			return err
		}
		resolved, err := analyticsContextFor(ctx, tx, h.now())
		if err != nil {
			return err
		}
		resolved.Timezone = frame.Timezone
		resolved.BaseCurrency = frame.BaseCurrency
		out = resolved
		return nil
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// analyticsContextFor assembles the caller's frame, minus the installation's
// zone and currency which the caller reads from the report frame.
func analyticsContextFor(ctx context.Context, tx pgx.Tx, now time.Time) (crmcontracts.AnalyticsContext, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return crmcontracts.AnalyticsContext{}, errors.New("compose: no actor bound to context")
	}

	def, defClause, err := AnalyticsPopulationClause(ctx, tx, RequestedScope{}, "", func(any) int { return 0 })
	if err != nil {
		return crmcontracts.AnalyticsContext{}, err
	}
	// The clause is discarded here on purpose: this route reports the decision,
	// it does not measure anything. Asking through the same door as the readers
	// is what keeps the offered default and the applied default one answer.
	_ = defClause

	allowed, err := allowedScopesFor(ctx, tx, p)
	if err != nil {
		return crmcontracts.AnalyticsContext{}, err
	}

	// Submitting a forecast is a create on the forecast object. Asked here so
	// the screen can hide the action rather than render a button whose save
	// returns 403 — the shape the review called out as advertising an action
	// the backend will reject.
	maySubmit := auth.Require(ctx, objectForecast, principal.ActionCreate) == nil

	return crmcontracts.AnalyticsContext{
		DefaultScope:  scopeToWire(def),
		AllowedScopes: allowed,
		Capabilities: crmcontracts.AnalyticsCapabilities{
			// A caller whose default population is their own records has no
			// manager forecast to look at: the manager forecast is an
			// assertion about a team or the workspace.
			ViewManagerForecast:   def.Kind != ScopeKindOwner,
			SubmitManagerForecast: maySubmit && def.Kind != ScopeKindOwner,
		},
		AsOf: now,
	}, nil
}

// allowedScopesFor enumerates what the screen may offer, in the order a control
// should show them: the widest population first, then the narrowings.
func allowedScopesFor(
	ctx context.Context, tx pgx.Tx, p principal.Principal,
) ([]crmcontracts.AnalyticsScope, error) {
	lens := p.Permissions.RowScope
	if auth.Unbounded(p) {
		lens = principal.RowScopeAll
	}

	me, err := analyticsUserLabel(ctx, tx, p.UserID)
	if err != nil {
		return nil, err
	}
	self := ids.UUID(p.UserID)
	mine := crmcontracts.AnalyticsScope{Kind: crmcontracts.AnalyticsScopeKindOwner, Label: me}
	mine.Id = ptrUUID(self)

	switch lens {
	case principal.RowScopeAll:
		out := []crmcontracts.AnalyticsScope{
			{Kind: crmcontracts.AnalyticsScopeKindWorkspace, Label: workspaceLabel},
		}
		teams, err := liveTeamsOf(ctx, tx, nil)
		if err != nil {
			return nil, err
		}
		return append(append(out, teams...), mine), nil

	case principal.RowScopeTeam:
		if len(p.TeamIDs) == 0 {
			return []crmcontracts.AnalyticsScope{mine}, nil
		}
		out := []crmcontracts.AnalyticsScope{
			{Kind: crmcontracts.AnalyticsScopeKindManagedTeams, Label: managedTeamsLabel},
		}
		teams, err := liveTeamsOf(ctx, tx, p.TeamIDs)
		if err != nil {
			return nil, err
		}
		return append(append(out, teams...), mine), nil

	default:
		return []crmcontracts.AnalyticsScope{mine}, nil
	}
}

// liveTeamsOf names the live teams, either all of them or a given set.
//
// Archived teams are absent rather than listed and refused later: an archived
// team is not a population, and offering one would produce a control whose
// every use is an error.
func liveTeamsOf(ctx context.Context, tx pgx.Tx, only []ids.UUID) ([]crmcontracts.AnalyticsScope, error) {
	sql := `SELECT id, name FROM team WHERE archived_at IS NULL`
	args := []any{}
	if only != nil {
		sql += ` AND id = ANY($1)`
		args = append(args, only)
	}
	sql += ` ORDER BY name`

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []crmcontracts.AnalyticsScope
	for rows.Next() {
		var id ids.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		scope := crmcontracts.AnalyticsScope{Kind: crmcontracts.AnalyticsScopeKindTeam, Label: name}
		scope.Id = ptrUUID(ids.UUID(id))
		out = append(out, scope)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scopeToWire(s ResolvedScope) crmcontracts.AnalyticsScope {
	out := crmcontracts.AnalyticsScope{
		Kind:  crmcontracts.AnalyticsScopeKind(s.Kind),
		Label: s.Label,
	}
	if s.ID != nil {
		out.Id = ptrUUID(ids.UUID(*s.ID))
	}
	return out
}

func ptrUUID(id ids.UUID) *openapi_types.UUID {
	out := openapi_types.UUID(id)
	return &out
}
