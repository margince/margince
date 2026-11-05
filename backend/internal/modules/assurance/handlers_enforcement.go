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

// PreviewFunc builds the preview. Compose supplies it, because it reads deals.
type PreviewFunc func(ctx context.Context) (EnforcementPreview, error)

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
	// The lock first: a setting the environment fixes says so, rather than
	// asking for a preview that would not unlock it either.
	if _, err := config.LockedChange(h.settings, QualifiedForecastEnforced,
		func(boot bool) bool { return boot == req.Enabled }); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if req.Enabled {
		if err := h.ensurePreviewCurrent(r.Context(), req.PreviewDigest); err != nil {
			httperr.Write(w, r, err)
			return
		}
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
	out := crmcontracts.ForecastAssuranceEnforcement{State: state}
	if h.locks != nil {
		out.Locks = h.locks([]config.Any{QualifiedForecastEnforced})
	}
	// The preview counts every open deal, so only the admin who may switch sees
	// it. A reader in a narrower scope would learn counts they cannot open.
	if h.preview != nil && state == crmcontracts.ForecastAssuranceEnforcementStateObserving &&
		auth.RequireAdmin(ctx) == nil {
		preview, err := h.preview(ctx)
		if err != nil {
			return out, err
		}
		out.Preview = &crmcontracts.ForecastEnforcementPreview{
			LeftOutCount: preview.LeftOut, UnknownCount: preview.Unknown, Digest: preview.Digest,
		}
	}
	return out, nil
}

// ensurePreviewCurrent refuses switching on unless the caller saw the preview
// that is true now. Switching on again while enforcing changes nothing, so it
// needs no preview.
func (h Handlers) ensurePreviewCurrent(ctx context.Context, seen *string) error {
	if h.preview == nil {
		return nil
	}
	on, err := config.StoredVal(ctx, h.settings, QualifiedForecastEnforced)
	if err != nil || on {
		return err
	}
	if err := auth.RequireAdmin(ctx); err != nil {
		return err
	}
	current, err := h.preview(ctx)
	if err != nil {
		return err
	}
	if seen == nil || *seen != current.Digest {
		return &httperr.DetailedError{
			Status: http.StatusConflict, Code: "preview_stale",
			Detail: "The deals this would leave out changed since you looked. " +
				"Check the new numbers, then switch it on again.",
		}
	}
	return nil
}
