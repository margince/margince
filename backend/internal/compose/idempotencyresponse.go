// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

const idempotencyConflict = "idempotency_key_conflict"

// writeClaimOutcome answers a claim that did not win the race: a replay
// (gated), a first attempt still in flight, or the same key reused for a
// different body.
func writeClaimOutcome(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, probes map[string]replayProbe, restore replayRestore, route string, outcome claimOutcome, stored storedResponse) {
	switch outcome {
	case claimReplay:
		// A replay is a read (API-CC-8): the recorded body only goes back if
		// the caller can still see the record it carries.
		if err := ensureReplayVisible(r.Context(), pool, probes, route, stored.body); err != nil {
			httperr.Write(w, r, err)
			return
		}
		if restore != nil {
			body, err := restore(r.Context(), route, stored.body)
			if err != nil {
				httperr.Write(w, r, err)
				return
			}
			stored.body = body
		}
		// Rehydrate capabilities after authorization, retaining the original status
		// and content type.
		if stored.contentType != "" {
			w.Header().Set("Content-Type", stored.contentType)
		}
		w.WriteHeader(stored.status)
		if stored.body != "" {
			if _, err := io.WriteString(w, stored.body); err != nil {
				slog.WarnContext(r.Context(), "idempotency response interrupted", "error", err)
			}
		}
	case claimInProgress:
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict,
			Code:   idempotencyConflict,
			Detail: "a request with this idempotency key is still in progress",
		})
	case claimMismatch:
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict,
			Code:   idempotencyConflict,
			Detail: "this idempotency key was already used with a different request body",
		})
	case claimFresh:
		// Unreachable: the caller returns early only for a non-fresh claim.
	case claimFailed:
		// Reached by two writers now: the tool surface records a run that
		// produced no result (agentidempotency.go), and this middleware records
		// a refusal whose handler had already committed (settleClaim). It
		// ANSWERS rather than falling through: an empty case here returns
		// without writing, and net/http then sends a bare 200 with no body — a
		// silent success for a call that failed.
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict,
			Code:   idempotencyConflict,
			Detail: "an earlier request with this idempotency key failed after it had already started; " +
				"check whether it took effect before retrying under a new key",
		})
	}
}
