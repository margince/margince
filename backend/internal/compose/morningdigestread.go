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
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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
	if err := refuseAnUnreadableDate(params); err != nil {
		httperr.Write(w, r, err)
		return
	}
	day := digestDay(params)
	payload, readErr := h.registry.ReadDigest(r.Context(), userID.UUID, day)
	if readErr != nil {
		// ReadDigest only touches Postgres and JSON — its failures are
		// storage faults, never the connector outage writeBackfillError's
		// default (502 provider_unreachable) would claim.
		h.log.ErrorContext(r.Context(), "digest read", "err", readErr)
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusInternalServerError, Code: "digest_read_failed",
			Detail: "The digest could not be read. Try again shortly.",
		})
		return
	}
	if payload == nil {
		httperr.Write(w, r, missingDigest(day))
		return
	}
	httperr.WriteJSON(w, http.StatusOK, payload)
}

// missingDigest tells a day that has no digest from an installation that has
// none at all.
//
// One message served both, so asking for a day the nightly run has not reached
// said the product had never built one.
func missingDigest(day *time.Time) *httperr.DetailedError {
	if day != nil {
		return &httperr.DetailedError{
			Status: http.StatusNotFound,
			Code:   "no_digest_for_date",
			Detail: "No digest was built for " + day.Format(time.DateOnly) +
				". Omit `date` for the most recent one.",
		}
	}
	return &httperr.DetailedError{
		Status: http.StatusNotFound,
		Code:   "no_digest_yet",
		Detail: "No digest has been built yet. The first nightly run creates it.",
	}
}

// refuseAnUnreadableDate rejects a `date` that names no day the store can hold.
//
// The store is queried with the day's ISO text, and Postgres has no year 0.
// That text must fall inside the range every request-stored date obeys.
// An empty parameter binds to the zero date, which no caller asked for.
func refuseAnUnreadableDate(params crmcontracts.GetMorningDigestParams) error {
	if params.Date == nil {
		return nil
	}
	if params.Date.IsZero() {
		return httperr.Validation("date", "invalid_date",
			"send `date` as a calendar day (YYYY-MM-DD), or omit it for the most recent digest.")
	}
	if !datasource.InstantInRange(params.Date.Time) {
		return httperr.Validation("date", "out_of_range",
			"that day is outside the years this product reads; send a recent calendar day (YYYY-MM-DD), or omit `date` for the most recent digest.")
	}
	return nil
}

// digestDay is the day a caller asked for, or nil for the most recent.
func digestDay(params crmcontracts.GetMorningDigestParams) *time.Time {
	if params.Date == nil {
		return nil
	}
	return &params.Date.Time
}
