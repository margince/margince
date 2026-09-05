// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Reading the chain a call leaves behind.
//
// A call supersedes rather than overwrites, so every number a period was ever
// given is already in the table — the write path has kept them since the
// forecast was first frozen. Nothing read them: CurrentCall takes the head and
// stops, which answers "what do we think now" and never "how did we get here".
//
// The second question is the one a review asks. A period that was called at
// 2.4M in April, 1.9M in May and 2.1M in June tells a story that its current
// figure of 2.1M does not, and the difference between those two readings is
// the whole reason the chain is kept.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// callHistoryLimit caps one period's history.
//
// Nothing bounds how often a period may be called — a manager can record a new
// number every minute, and the chain keeps every one — so an uncapped read is
// a response that grows without limit. The cap is deliberately far above what
// a period accumulates in practice: a reviewer scrolling a quarter's calls is
// reading tens of entries, not hundreds.
//
// It takes the NEWEST entries, so a period that somehow exceeded it still
// answers the recent history a reader is actually asking about rather than
// stopping at the oldest.
const callHistoryLimit = 500

// callHistoryTx is the read itself, so a caller already holding a transaction
// reads the history in the same one as the readings beside it. Ungated for the
// reason standingCallTx is: its callers gate, and gating twice would refuse a
// seat this product does not have.
func (s *Store) callHistoryTx(ctx context.Context, tx pgx.Tx, period Period, scope Scope) ([]Call, error) {
	// IS NOT DISTINCT FROM rather than =, because the workspace scope's id is
	// NULL and `scope_id = NULL` is never true. Written with =, the one scope
	// every installation has would report an empty history however many calls
	// it holds.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id, period_start, period_end, scope_kind, scope_id,
		       amount_minor, currency, note, author_id, supersedes_id, created_at
		FROM forecast_call
		WHERE period_start = $1 AND period_end = $2
		  AND scope_kind = $3 AND scope_id IS NOT DISTINCT FROM $4
		ORDER BY created_at DESC, id DESC
		LIMIT %d`, callHistoryLimit),
		period.StartDate, period.EndDate, scope.Kind, scope.ID)
	if err != nil {
		return nil, fmt.Errorf("forecasting: reading the call history: %w", err)
	}
	defer rows.Close()

	// Empty rather than nil: a period nobody has called serves `"calls": []`,
	// and a caller cannot tell that from `"calls": null` without knowing which
	// of the two this code happened to produce.
	out := []Call{}
	for rows.Next() {
		var call Call
		var note *string
		if err := rows.Scan(&call.ID, &call.PeriodStart, &call.PeriodEnd,
			&call.Scope.Kind, &call.Scope.ID, &call.AmountMinor, &call.Currency,
			&note, &call.AuthorID, &call.SupersedesID, &call.CreatedAt); err != nil {
			return nil, fmt.Errorf("forecasting: reading a call: %w", err)
		}
		if note != nil {
			call.Note = *note
		}
		out = append(out, call)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("forecasting: collecting the call history: %w", err)
	}
	return out, nil
}

// ListForecastCalls answers what this period's forecast has been called.
//
// The scope is resolved through the SAME seam the readings use, and for the
// same reason: a call names a population, and answering for a population the
// caller may not read hands them a number about deals they cannot see. The
// seam reads the deals to resolve it, which is more work than a history needs
// — and the alternative is a second scope resolver whose answer could differ
// from the readings' beside it, which is the drift worth paying to avoid.
func (h Handlers) ListForecastCalls(
	w http.ResponseWriter, r *http.Request, params crmcontracts.ListForecastCallsParams,
) {
	var spelled string
	if params.ScopeKind != nil {
		spelled = string(*params.ScopeKind)
	}
	scope, err := readScope(spelled, params.ScopeId)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	at := h.now()
	if params.AsOf != nil {
		at = DayNamed(params.AsOf.Time)
	}
	asked := ""
	if params.Period != nil {
		asked = string(*params.Period)
	}
	kind, known := PeriodKindOf(asked)
	if !known {
		httperr.Write(w, r, unknownPeriod())
		return
	}

	calls := []Call{}
	err = h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		period, baseCurrency, err := h.period(ctx, tx, kind, at)
		if err != nil {
			return err
		}
		_, resolved, _, err := h.deals(ctx, tx, period, scope, at, baseCurrency)
		if err != nil {
			return err
		}
		// The managed-teams reading covers several populations at once, and a
		// call is an assertion about ONE. There is no chain to read for it —
		// the same reason GetForecast looks up no standing call under it.
		if resolved.Kind == ScopeManagedTeams {
			return nil
		}
		calls, err = h.store.callHistoryTx(ctx, tx, period, resolved)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}

	wire := make([]crmcontracts.ForecastCall, 0, len(calls))
	for _, call := range calls {
		wire = append(wire, callToWire(call))
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Calls []crmcontracts.ForecastCall `json:"calls"`
	}{Calls: wire})
}
