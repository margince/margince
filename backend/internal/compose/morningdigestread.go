// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The morning digest read.
//
// It shares backfillHandlers with the backfill ops because it shares their one
// dependency — the capture registry — and nothing else: a digest is what the
// nightly build already assembled, not a window somebody is about to spend on.
// Its own file for that reason, and because the four backfill ops now share a
// preflight it does not want (it names no provider, so there is no connection
// kind to check).

import (
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// GetMorningDigest serves the caller's stored digest (CAP-WIRE-6): one
// indexed row, pre-assembled by the nightly build — no digest yet is the
// honest 404, never a fabricated empty payload.
func (h backfillHandlers) GetMorningDigest(w http.ResponseWriter, r *http.Request, params crmcontracts.GetMorningDigestParams) {
	if !h.backfillWired(w, r, "GetMorningDigest") {
		return
	}
	userID, ok := h.caller(w, r)
	if !ok {
		return
	}
	var day *time.Time
	if params.Date != nil {
		day = &params.Date.Time
	}
	payload, err := h.registry.ReadDigest(r.Context(), userID.UUID, day)
	if err != nil {
		// ReadDigest only touches Postgres and JSON — its failures are
		// storage faults, never the connector outage writeBackfillError's
		// default (502 provider_unreachable) would claim.
		h.log.ErrorContext(r.Context(), "digest read", "err", err)
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError, Code: "digest_read_failed",
			Detail: "The digest could not be read. Try again shortly.",
		})
		return
	}
	if payload == nil {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusNotFound,
			Code:   "no_digest_yet",
			Detail: "No digest has been built yet — the first nightly run creates it.",
		})
		return
	}
	httperr.WriteJSON(w, http.StatusOK, payload)
}
