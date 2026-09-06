// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

// The HTTP transport for the account scan. Wire concerns only: bind the path
// id, say whether this is a read or an ensure, and hand the result to the
// sentinel error mapping. The service owns every other gate.

import (
	"context"
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OverlayMode answers whether the calling workspace reads from an incumbent
// mirror instead of this system of record.
type OverlayMode func(ctx context.Context) (bool, error)

// Handlers shadows the generated scan stubs.
type Handlers struct {
	svc     *Service
	overlay OverlayMode
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service, overlay OverlayMode) Handlers {
	return Handlers{svc: svc, overlay: overlay}
}

// GetOrganizationScan implements GET /organizations/{id}/scan.
func (h Handlers) GetOrganizationScan(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.native(w, r) {
		return
	}
	scan, err := h.svc.Get(r.Context(), ids.From[ids.OrganizationKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, scan)
}

// EnsureOrganizationScan implements POST /organizations/{id}/scan. The body
// is optional: an open with nothing to say sends none.
func (h Handlers) EnsureOrganizationScan(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.native(w, r) {
		return
	}
	var req crmcontracts.OrganizationScanRequest
	if r.ContentLength != 0 && !httperr.Decode(w, r, &req) {
		return
	}
	force := req.Force != nil && *req.Force
	scan, err := h.svc.Ensure(r.Context(), ids.From[ids.OrganizationKind](ids.UUID(id)), force)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, scan)
}

// native reports whether this workspace reads from this system of record,
// writing the refusal itself when it does not. The scan is read from native
// rows — the 360 and the account's own messages — and a workspace reading
// from an incumbent mirror has neither, so the honest answer is that the
// surface is unavailable in this mode rather than a confident reading of an
// account whose records live somewhere else.
func (h Handlers) native(w http.ResponseWriter, r *http.Request) bool {
	if h.overlay == nil {
		// An unwired mode check is a deployment defect on a surface whose
		// whole premise is which system of record it reads. It refuses as OUR
		// fault: nothing about the request is wrong.
		httperr.Write(w, r, errors.New(
			"the account scan cannot confirm which system of record this workspace reads from"))
		return false
	}
	overlay, err := h.overlay(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return false
	}
	if overlay {
		httperr.Write(w, r, httperr.Validation("id", "unsupported_in_overlay_mode",
			"the account scan reads this system of record's own messages and pipeline; while the "+
				"workspace reads from the incumbent mirror there is nothing here to read — open the "+
				"account in the incumbent's own UI"))
		return false
	}
	return true
}
