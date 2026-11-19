// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// StateCall puts the standing call this seat sees on the readings, and answers
// it for the landing. A seat that may not see calls gets call_withheld, so no
// client tells it that nobody has called. GET /forecast and a live share both
// state the call through it.
func StateCall(
	ctx context.Context, tx pgx.Tx, store *Store, mayAssert bool,
	period Period, scope Scope, out *crmcontracts.ForecastReadings,
) (SeenCall, error) {
	call, seen, err := CallSeen(ctx, tx, store, mayAssert, period, scope)
	if err != nil {
		return SeenCall{}, err
	}
	if seen.Withheld {
		out.CallWithheld = new(true)
	}
	if seen.Amount != nil {
		wire := callToWire(call)
		out.CurrentCall = &wire
	}
	return seen, nil
}

// CallSeen is the standing call a seat sees for a resolved population. It is
// withheld when the seat may not assert about it. It is also withheld when the
// reading covers several populations (managed teams). It is empty when nobody
// has called. Every surface that states a call or a landing reads the call through
// it, so they agree on one landing and one caveat.
func CallSeen(ctx context.Context, tx pgx.Tx, store *Store, mayAssert bool, period Period, scope Scope) (Call, SeenCall, error) {
	if !mayAssert || scope.Kind == ScopeManagedTeams {
		return Call{}, SeenCall{Withheld: true}, nil
	}
	call, err := store.CurrentCallTx(ctx, tx, period, scope)
	if IsNoStandingCall(err) {
		return Call{}, SeenCall{}, nil
	}
	if err != nil {
		return Call{}, SeenCall{}, err
	}
	return call, SeenCall{Amount: &call.AmountMinor}, nil
}

// FrozenCallSeen is CallSeen for a snapshot share. A snapshot keeps the amount
// of the call its landing used, never the call record, so the share never
// states a record. The first answer is the call the landing takes. noneStood
// is true only where the snapshot shows nobody had called: a manager-call
// setting over every pipeline that kept a landing and no call amount. Any
// other snapshot cannot tell, so the share withholds the call.
func FrozenCallSeen(mayAssert bool, frozen FrozenCall) (landing SeenCall, noneStood bool) {
	if !mayAssert {
		return SeenCall{Withheld: true}, false
	}
	recorded := frozen.LandingMeasure != nil && frozen.ForwardMeasure != nil &&
		ForwardMeasure(*frozen.ForwardMeasure) == MeasureManagerCall && !frozen.OnePipeline
	if !recorded && frozen.CallMinor == nil {
		return SeenCall{Withheld: true}, false
	}
	return SeenCall{Amount: frozen.CallMinor}, frozen.CallMinor == nil
}

// FrozenCall is what a snapshot kept about the call its landing used. It holds
// the forward measure setting, the landing's measure, whether one pipeline was
// read, and the call amount.
type FrozenCall struct {
	ForwardMeasure, LandingMeasure *string
	OnePipeline                    bool
	CallMinor                      *int64
}
