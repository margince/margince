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
// precise half. An operation the table does not place is in no area, like an
// unplaced tool. Only a passport set to all areas runs it.
func operationAreaRefusal(ctx context.Context, tool, operationID string) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return nil
	}
	spec := mcp.ToolSpec{Name: tool}
	if area, placed := workarea.OfOperation(operationID); placed {
		spec.Areas = []string{area}
	}
	return auth.AreaRefusal(p, spec)
}

// refusedOutsideAreas answers an agent's REST call on a route its passport's
// areas leave off, and reports whether it did. A route an agent may not call
// at all is left to the checks after it, which refuse it with their own reason.
func refusedOutsideAreas(w http.ResponseWriter, r *http.Request) bool {
	pattern := chi.RouteContext(r.Context()).RoutePattern()
	pol, admitted := routeAdmitsAgents(r.Method, pattern)
	if !admitted {
		return false
	}
	if err := operationAreaRefusal(r.Context(), "", pol.Op); err != nil {
		httperr.Write(w, r, err)
		return true
	}
	return false
}
