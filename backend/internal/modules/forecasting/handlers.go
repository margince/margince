// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// DealsFunc reads the deals in scope for a period, already row-scoped.
//
// Injected rather than imported: the deals live in another module and the row
// scope is applied where the caller's authority already sits, which is the
// composition layer. The boolean says whether anything was withheld — and it is
// a boolean and never a count, because a count of what a caller may not read is
// itself a statement about how much of it there is.
//
// asOf and baseCurrency travel with the request because the conversion needs
// both: a rate is looked up as of a DAY, and there is no base amount without a
// currency to convert into.
// The Scope it returns is the one it RESOLVED, which is not always the one it
// was handed: an unset scope means the caller named nothing, and what that
// means depends on their lens. The reading reports the resolved one, so an
// answer always says which population it is about.
type DealsFunc func(ctx context.Context, tx pgx.Tx, period Period, scope Scope, asOf time.Time, baseCurrency string) ([]Deal, Scope, bool, error)

// PeriodFunc resolves the installation's window for a day, and the currency its
// money is counted in. Injected for the same reason: the fiscal settings belong
// to identity.
type PeriodFunc func(ctx context.Context, tx pgx.Tx, kind PeriodKind, at time.Time) (Period, string, error)

// ResolveScopeFunc answers the population this caller may measure, refusing one
// they may not. Injected, because deciding it reads teams and seats and this
// module owns neither.
type ResolveScopeFunc func(ctx context.Context, tx pgx.Tx, requested Scope) (Scope, error)

// HistoryFunc reads the conversion evidence a sufficiency answer rests on.
//
// A seam for the same reason DealsFunc is: the rates come from tables this
// module does not own, and the composition applies the caller's row scope
// before any of it reaches the arithmetic.
type HistoryFunc func(ctx context.Context, tx pgx.Tx, period Period, scope Scope, asOf time.Time, baseCurrency string) (ConversionHistory, error)

// HoldsDealsFunc answers whether the population a reading is about holds any
// deal at all, with the same population clause the reading's deals used.
type HoldsDealsFunc func(ctx context.Context, tx pgx.Tx, scope Scope) (bool, error)

// MeasureFunc reads which remaining-pipeline reading this installation builds
// its landing from.
type MeasureFunc func(ctx context.Context, tx pgx.Tx) (ForwardMeasure, error)

// Handlers is the forecast's HTTP surface.
type Handlers struct {
	store   *Store
	deals   DealsFunc
	period  PeriodFunc
	resolve ResolveScopeFunc
	history HistoryFunc
	measure MeasureFunc
	holds   HoldsDealsFunc
	now     func() time.Time
}

// NewHandlers binds the routes to the store and its seams.
func NewHandlers(
	store *Store, deals DealsFunc, period PeriodFunc, resolve ResolveScopeFunc,
	history HistoryFunc, measure MeasureFunc, holds HoldsDealsFunc, now func() time.Time,
) Handlers {
	return Handlers{
		store: store, deals: deals, period: period, resolve: resolve,
		history: history, measure: measure, holds: holds, now: now,
	}
}

// GetForecast answers the readings for one period.
//
// The period resolution, the deal read and the standing call all happen in ONE
// transaction. Split across several, a settings change mid-request would label
// one period's total with another period's frame.
func (h Handlers) GetForecast(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetForecastParams,
) {
	scope, err := scopeFromParams(params.ScopeKind, params.ScopeId)
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

	var out crmcontracts.ForecastReadings
	err = h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		period, baseCurrency, err := h.period(ctx, tx, kind, at)
		if err != nil {
			return err
		}
		deals, resolved, limited, err := h.deals(ctx, tx, period, scope, at, baseCurrency)
		if err != nil {
			return err
		}
		scope = resolved
		// The as-of DAY, not the instant. The slipped rule compares calendar
		// days, and handing it a clock makes a deal due today read as slipped
		// from noon onward — which the report engine, comparing dates, would
		// not agree with.
		readings, err := Compute(period, period.LocalDay(at), deals)
		if err != nil {
			return err
		}
		out = ReadingsToWire(period, scope, readings, baseCurrency, at)
		out.ScopeLimited = &limited
		holds, err := h.holds(ctx, tx, scope)
		if err != nil {
			return err
		}
		out.ScopeHoldsDeals = &holds
		// Totals are readable more widely than a standing call is. The call,
		// its author and its snapshots assert something about a population.
		// Only a seat that may assert about it sees them.
		mayAssert, err := MayAssertAbout(ctx, tx, h.resolve, scope)
		if err != nil {
			return err
		}
		if mayAssert {
			refs, err := h.store.SnapshotRefsTx(ctx, tx, period, scope, baseCurrency)
			if err != nil {
				return err
			}
			out.Snapshots = snapshotRefsToWire(refs)
		}

		// A standing call is an assertion about ONE named population. The
		// managed-teams reading covers several, so there is no call to look up
		// — and looking one up under a flattened scope would fetch a different
		// population's call and print it beside these totals.
		if scope.Kind == ScopeManagedTeams {
			return nil
		}
		var called *int64
		call, err := h.store.CurrentCallTx(ctx, tx, period, scope)
		switch {
		case !mayAssert:
			// The call exists for others; this seat is shown none.
		case err == nil:
			wire := callToWire(call)
			out.CurrentCall = &wire
			called = &call.AmountMinor
		case IsNoStandingCall(err):
			// Nobody has called this period. A real answer, and absent rather
			// than an error.
		default:
			return err
		}
		return h.project(ctx, tx, &out, period, scope, readings, called, at, baseCurrency)
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// MayAssertAbout asks the write authority whether this seat may see the
// standing call and the snapshots of a resolved population. A refusal hides
// them; any other failure is real. The forecast endpoint and the forecast_readings
// tool both ask it.
func MayAssertAbout(ctx context.Context, tx pgx.Tx, resolve ResolveScopeFunc, scope Scope) (bool, error) {
	if scope.Kind == ScopeManagedTeams {
		return true, nil
	}
	_, err := resolve(ctx, tx, scope)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
		return false, nil
	default:
		return false, err
	}
}

// project adds where the period lands and whether the pipeline supports it.
//
// Separate from GetForecast because it is a second question over the same
// readings, and because both halves must be able to fail SOFTLY: a landing this
// installation cannot build, or a history too thin to divide by, is an answer —
// the reading itself is still true and still worth returning.
func (h Handlers) project(
	ctx context.Context, tx pgx.Tx, out *crmcontracts.ForecastReadings,
	period Period, scope Scope, readings Readings, call *int64,
	asOf time.Time, baseCurrency string,
) error {
	measure, err := h.measure(ctx, tx)
	if err != nil {
		return err
	}
	landing, err := ProjectLanding(readings, measure, call)
	if err != nil {
		return err
	}
	wire := landingToWire(landing)
	out.Landing = &wire

	// The managed-teams reading covers several populations, so a conversion
	// rate over it would blend books that are measured separately — and the
	// coverage figure it produced would describe none of them.
	if scope.Kind == ScopeManagedTeams {
		return nil
	}
	history, err := h.history(ctx, tx, period, scope, asOf, baseCurrency)
	if err != nil {
		return err
	}
	sufficiency, err := AssessSufficiency(readings, history, call)
	if err != nil {
		return err
	}
	assessed := sufficiencyToWire(sufficiency)
	out.Sufficiency = &assessed
	return nil
}

// landingToWire maps a projection onto the wire shape.
//
// The caveat is omitted rather than sent empty when there is none: an empty
// string would render as a warning that says nothing.
func landingToWire(in Landing) crmcontracts.ForecastLanding {
	out := crmcontracts.ForecastLanding{
		AmountMinor:    in.AmountMinor,
		Measure:        crmcontracts.ForecastLandingMeasure(in.Measure),
		WonMinor:       in.WonMinor,
		RemainingMinor: in.RemainingMinor,
		SystemMinor:    in.SystemMinor,
	}
	if in.Caveat != "" {
		caveat := crmcontracts.ForecastLandingCaveat(in.Caveat)
		out.Caveat = &caveat
	}
	return out
}

// sufficiencyToWire maps an assessment onto the wire shape.
//
// An absence carries ONLY its reason. Sending the zeroed figures beside it
// would let a client draw "0 needed, 0 covered" — which reads as a fully
// covered pipeline and is the opposite of what an absence means.
func sufficiencyToWire(in Sufficiency) crmcontracts.ForecastSufficiency {
	if in.Absent != "" {
		absent := crmcontracts.ForecastSufficiencyAbsent(in.Absent)
		return crmcontracts.ForecastSufficiency{Absent: &absent}
	}
	basis := crmcontracts.ForecastSufficiencyBasis(in.Basis)
	return crmcontracts.ForecastSufficiency{
		Basis:                   &basis,
		ReferenceLandingMinor:   &in.ReferenceLandingMinor,
		RemainingToSupportMinor: &in.RemainingToSupportMinor,
		NeededOpenMinor:         &in.NeededOpenMinor,
		CurrentOpenMinor:        &in.CurrentOpenMinor,
		CoverageBp:              &in.CoverageBp,
	}
}

// RecordForecastCall records what somebody believes will close.
func (h Handlers) RecordForecastCall(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.NewForecastCall
	if !httperr.Decode(w, r, &body) {
		return
	}
	scope, err := callScopeFromBody(body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	at := h.now()
	if body.AsOf != nil {
		at = DayNamed(body.AsOf.Time)
	}
	askedPeriod := ""
	if body.Period != nil {
		askedPeriod = string(*body.Period)
	}
	kind, known := PeriodKindOf(askedPeriod)
	if !known {
		httperr.Write(w, r, unknownPeriod())
		return
	}

	var out Call
	err = h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		period, _, err := h.period(ctx, tx, kind, at)
		if err != nil {
			return err
		}
		// WHICH population this call is about, resolved against the caller's
		// own authority rather than taken from the body.
		//
		// The RBAC gate asks whether this seat may create a forecast call, not
		// which population they may assert one for — and `scope_kind` defaults
		// to workspace, so a team-lens manager who sent no scope at all was
		// creating the whole installation's standing forecast, and superseding
		// whatever management had asserted. The editor is hidden for them in
		// the app, which is a courtesy and not a check.
		//
		// Inside the transaction, so the authority that admitted the write is
		// the authority that held when it committed.
		allowed, err := h.resolve(ctx, tx, scope)
		if err != nil {
			return err
		}
		out, err = h.store.RecordCallTx(ctx, tx, NewCall{
			Period: period, Scope: allowed, AmountMinor: body.AmountMinor,
			Currency: body.Currency, Note: derefString(body.Note),
		})
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, callToWire(out))
}

// scopeFromParams turns the two query keys into one scope, refusing the pairs
// the table's own CHECK would refuse — so a caller reads a named field back
// rather than a constraint violation.
func scopeFromParams(
	kind *crmcontracts.GetForecastParamsScopeKind, id *openapi_types.UUID,
) (Scope, error) {
	var spelled string
	if kind != nil {
		spelled = string(*kind)
	}
	return readScope(spelled, id)
}

// readScope is the READ door's rule, spelled once for the endpoints that take
// a scope off the query string. Each arrives carrying its OWN generated
// scope_kind type for the same three values, so the shared rule takes the
// string they both convert to — a copy per endpoint is how one comes to admit
// a scope the other refuses.
//
// Held by: TestTheReadScopeRuleHasOneSpelling (scope_test.go)
//
// Distinct from callScopeFromBody below, which is the WRITE door and defaults
// an omission to the workspace. Here an omission stays unset for the seam to
// resolve against the caller's own lens, because a reader who names no scope
// is asking about whatever they can see rather than about the whole company.
func readScope(kind string, id *openapi_types.UUID) (Scope, error) {
	scope := Scope{Kind: kind}
	if id != nil {
		asID := ids.UUID(*id)
		scope.ID = &asID
	}
	// An omission is carried through as unset for the seam to resolve against
	// the caller's own lens. Naming an id without a kind is still malformed,
	// and says so rather than being read as one of the named scopes.
	if scope.Kind == ScopeUnset {
		if scope.ID != nil {
			return Scope{}, &values.ParseError{
				Field: colScopeKind, Code: "required",
				Message: "a scope_id names whose forecast, so it needs a scope_kind beside it",
			}
		}
		return scope, nil
	}
	if err := checkScope(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// callScopeFromBody is the same rule for the request body, whose scope_kind is
// its own generated type.
func callScopeFromBody(body crmcontracts.NewForecastCall) (Scope, error) {
	scope := Scope{Kind: ScopeWorkspace}
	if body.ScopeKind != nil {
		scope.Kind = string(*body.ScopeKind)
	}
	if body.ScopeId != nil {
		asID := ids.UUID(*body.ScopeId)
		scope.ID = &asID
	}
	if err := checkScope(scope); err != nil {
		return Scope{}, err
	}
	return scope, nil
}
