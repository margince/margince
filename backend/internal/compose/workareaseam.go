// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/workarea"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// reachableAreasOf answers which areas a passport may be limited to from the
// registry the server serves, so the choice offered is the tool list's.
func reachableAreasOf(reg *agents.Registry) identity.ReachableAreas {
	return func(ctx context.Context) []string {
		p, ok := principal.Actor(ctx)
		if !ok || reg == nil {
			return nil
		}
		return reg.ReachableAreas(p)
	}
}

// operationAreaRefusal holds an agent to the one operation's area. Gate.Admit
// admits a verb in several areas while any of them is on, and this is the
// precise half.
func operationAreaRefusal(ctx context.Context, tool, operationID string) error {
	p, ok := principal.Actor(ctx)
	area, placed := workarea.OfOperation(operationID)
	if !ok || !placed {
		return nil
	}
	return auth.AreaRefusal(p, mcp.ToolSpec{Name: tool, Areas: []string{area}})
}

// refusedOutsideAreas answers an agent's REST call on a route its passport's
// areas leave off, and reports whether it did. A route no policy places is left
// to the checks after it, which refuse an unknown one.
func refusedOutsideAreas(w http.ResponseWriter, r *http.Request) bool {
	pattern := chi.RouteContext(r.Context()).RoutePattern()
	pol := agentPolicies[r.Method+" "+pattern]
	if pol.Op == "" {
		return false
	}
	if err := operationAreaRefusal(r.Context(), "", pol.Op); err != nil {
		httperr.Write(w, r, err)
		return true
	}
	return false
}
