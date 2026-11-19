// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// StateCall puts the standing call this seat sees on the readings, and answers
// its amount for the landing. A seat that may not see calls gets call_withheld,
// so no client tells it that nobody has called. GET /forecast and a live share
// both state the call through it.
func StateCall(
	ctx context.Context, tx pgx.Tx, store *Store, mayAssert bool,
	period Period, scope Scope, out *crmcontracts.ForecastReadings,
) (*int64, error) {
	if !mayAssert {
		out.CallWithheld = new(true)
		return nil, nil
	}
	call, seen, err := CallSeen(ctx, tx, store, mayAssert, period, scope)
	if err != nil || !seen {
		return nil, err
	}
	wire := callToWire(call)
	out.CurrentCall = &wire
	return &call.AmountMinor, nil
}

// requireCalledAmount refuses a call whose body left amount_minor out or sent
// null. Both decode to 0, and a call of 0 is a real number somebody typed.
func requireCalledAmount(r *http.Request) error {
	if amount, _ := httperr.PresentField(r, "amount_minor"); amount == nil {
		return httperr.Validation("amount_minor", "required",
			"amount_minor is required: send the amount you expect to close, or 0 for a call of nothing")
	}
	return nil
}
