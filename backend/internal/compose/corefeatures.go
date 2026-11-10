// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/corefeatures"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/workarea"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type coreFeatureSettings struct {
	listsEnabled bool
	coreFeatures corefeatures.Availability
}

// WithDisabledCoreFeatures binds the same feature decision to HTTP and MCP.
func WithDisabledCoreFeatures(disabled []string) Option {
	availability := corefeatures.New(disabled)
	return func(s *Server, _ *pgxpool.Pool) { s.coreFeatures = availability }
}

func (s *Server) publishCoreFeatures() {
	s.listsEnabled = s.listsEnabled && s.coreFeatures.Enabled(crmcontracts.CoreFeatureFilters)
	s.authHandlers = s.WithCoreFeatures(s.coreFeatures)
}

func coreOperationAvailable(availability corefeatures.Availability, policy agentPolicy) bool {
	area, placed := workarea.PlacementOfOperation(policy.Op)
	if !placed {
		return availability.RecordEnabled(string(policy.RecordType))
	}
	return availability.AreaEnabled(area) && (!corefeatures.AreaUsesRecordScope(area) || availability.RecordEnabled(string(policy.RecordType)))
}

func coreFeatureGate(availability corefeatures.Availability) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := chi.RouteContext(r.Context())
			if route == nil {
				next.ServeHTTP(w, r.WithContext(availability.Context(r.Context())))
				return
			}
			pattern := route.RoutePattern()
			policy := agentPolicies[r.Method+" "+pattern]
			if !coreOperationAvailable(availability, policy) || !availability.RecordEnabled(chi.URLParam(r, "record_type")) {
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			next.ServeHTTP(w, r.WithContext(availability.Context(r.Context())))
		})
	}
}

type coreCallPolicy struct{ corefeatures.Availability }

func coreCallAvailability(availability corefeatures.Availability) coreCallPolicy {
	return coreCallPolicy{availability}
}

func (p coreCallPolicy) Admits(verb, recordType string) bool {
	areas := workarea.PlacementsOfVerb(verb)
	if len(areas) == 1 && !corefeatures.AreaUsesRecordScope(areas[0]) {
		return p.AreaEnabled(areas[0])
	}
	if !p.RecordEnabled(recordType) {
		return false
	}
	if len(areas) == 0 {
		return true
	}
	return slices.ContainsFunc(areas, p.AreaEnabled)
}

func coreReadViews(availability corefeatures.Availability) []agents.ReadView {
	views := readViews(agentPolicies)
	kept := make([]agents.ReadView, 0, len(views))
	for _, view := range views {
		if coreOperationAvailable(availability, agentPolicies[http.MethodGet+" "+view.Pattern]) {
			kept = append(kept, view)
		}
	}
	return kept
}
