// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WithSettings wires the configuration the enforcement switch is read and
// saved through.
func (h Handlers) WithSettings(cfg *config.Config) Handlers {
	h.settings = cfg
	return h
}

// GetForecastAssuranceEnforcement answers whether the qualified forecast
// applies the viability reviews.
func (h Handlers) GetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		httperr.NotImplemented(w, r, "GetForecastAssuranceEnforcement")
		return
	}
	if err := auth.Require(r.Context(), "forecast", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	state, err := h.enforcementState(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, state)
}

// SetForecastAssuranceEnforcement switches enforcement on or off.
func (h Handlers) SetForecastAssuranceEnforcement(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		httperr.NotImplemented(w, r, "SetForecastAssuranceEnforcement")
		return
	}
	var req crmcontracts.SetForecastAssuranceEnforcementJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.SetQualifiedForecastEnforced(r.Context(), h.settings, req.Enabled); err != nil {
		httperr.Write(w, r, err)
		return
	}
	state, err := h.enforcementState(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, state)
}

func (h Handlers) enforcementState(ctx context.Context) (crmcontracts.ForecastAssuranceEnforcement, error) {
	started, err := h.store.EverRun(ctx)
	if err != nil {
		return crmcontracts.ForecastAssuranceEnforcement{}, err
	}
	on, err := config.StoredVal(ctx, h.settings, QualifiedForecastEnforced)
	if err != nil {
		return crmcontracts.ForecastAssuranceEnforcement{}, err
	}
	state := crmcontracts.ForecastAssuranceEnforcementStateNotEnrolled
	switch {
	case started && on:
		state = crmcontracts.ForecastAssuranceEnforcementStateEnforcing
	case started:
		state = crmcontracts.ForecastAssuranceEnforcementStateObserving
	}
	return crmcontracts.ForecastAssuranceEnforcement{State: state}, nil
}
