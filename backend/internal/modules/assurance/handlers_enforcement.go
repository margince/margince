// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

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

// WithLocks binds what reports the settings a boot source sets.
func (h Handlers) WithLocks(locks func(surface []config.Any) *[]crmcontracts.SettingLock) Handlers {
	h.locks = locks
	return h
}

// EnforcementPreview is what switching enforcement on would do now. It counts
// the open deals the qualified forecast would leave out and those it could not
// judge, and digests every deal's treatment. Switching on is bound to the
// digest, so nobody switches on a picture that has since changed.
type EnforcementPreview struct {
	LeftOut int
	Unknown int
	Digest  string
}

// PreviewFunc builds the preview inside the caller's transaction. Compose
// supplies it, because it reads deals.
type PreviewFunc func(ctx context.Context, tx pgx.Tx) (EnforcementPreview, error)

// WithPreview binds what previews switching enforcement on.
func (h Handlers) WithPreview(preview PreviewFunc) Handlers {
	h.preview = preview
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
	binding := PreviewBinding{Seen: req.PreviewDigest, Preview: h.preview}
	if err := h.store.SetQualifiedForecastEnforced(r.Context(), h.settings, req.Enabled, binding); err != nil {
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
	out := crmcontracts.ForecastAssuranceEnforcement{State: state}
	if h.locks != nil {
		out.Locks = h.locks([]config.Any{QualifiedForecastEnforced})
	}
	// The preview counts every open deal, so only the admin who may switch sees
	// it. A reader in a narrower scope would learn counts they cannot open.
	if h.preview != nil && state == crmcontracts.ForecastAssuranceEnforcementStateObserving &&
		auth.RequireAdmin(ctx) == nil {
		preview, err := h.store.ReadPreview(ctx, h.preview)
		if err != nil {
			return out, err
		}
		out.Preview = &crmcontracts.ForecastEnforcementPreview{
			LeftOutCount: preview.LeftOut, UnknownCount: preview.Unknown, Digest: preview.Digest,
		}
	}
	return out, nil
}
