// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The transport for a rep's own answer to "how much of this queue should answer
// itself". Apart from handlers.go because it is a different surface: that file
// carries the inbox — a list of things other people and machines proposed — and
// this one carries a setting the reader holds about themselves.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// GetAutonomy answers with the reader's own settings, one row per eligible kind.
func (h Handlers) GetAutonomy(w http.ResponseWriter, r *http.Request) {
	// Human-only, matching the contract. An agent asking what its principal has
	// automated is asking a question about the person rather than about the work,
	// and the answer would tell it which proposals it can expect to skip review.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	settings, err := h.svc.AutoApplySettings(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, autonomyWire(settings))
}

// UpdateAutonomy turns one kind on or off for the reader, then answers with the
// whole set.
//
// It returns everything rather than the row it changed so the client has no
// second read to get wrong: a screen that patched one row and re-rendered from
// its own optimistic copy would show a stale track record beside the switch the
// reader just moved.
func (h Handlers) UpdateAutonomy(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.UpdateAutonomyRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.svc.SetAutoApply(r.Context(), req.Kind, req.Auto); err != nil {
		httperr.Write(w, r, err)
		return
	}
	settings, err := h.svc.AutoApplySettings(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, autonomyWire(settings))
}

// autonomyWire renders the service's rows onto the contract's shape.
func autonomyWire(settings []KindAutonomy) crmcontracts.AutonomySettings {
	data := make([]crmcontracts.KindAutonomy, 0, len(settings))
	for _, s := range settings {
		data = append(data, crmcontracts.KindAutonomy{
			Kind:           s.Kind,
			Mode:           crmcontracts.KindAutonomyMode(s.Mode),
			ApprovedClean:  s.ApprovedClean,
			ApprovedEdited: s.ApprovedEdited,
			Rejected:       s.Rejected,
		})
	}
	return crmcontracts.AutonomySettings{Data: data}
}
