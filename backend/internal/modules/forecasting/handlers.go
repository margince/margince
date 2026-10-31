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

// Assembly is one forecast result: the readings with their per-deal rows, the
// scope they resolved to, and the basis the chances stood on.
type Assembly struct {
	Readings Readings
	// Scope is the population RESOLVED, which an unset request leaves to the
	// caller's lens; the answer always names it.
	Scope Scope
	// Limited says deals the caller cannot read were left out. A boolean and
	// never a count, because a count of what a caller may not read is itself a
	// statement about how much of it there is.
	Limited bool
	// SelfCorrecting is whether the chances moved with recorded outcomes, and
	// EvidenceCutoff the instant no later outcome counted.
	SelfCorrecting bool
	EvidenceCutoff time.Time
	// RiskSignalsVersion names the signal rules on the rows; empty while
	// self-correction is off and no signal was assessed.
	RiskSignalsVersion string
}

// AssembleFunc builds the forecast result for a period, row-scoped. Injected
// rather than imported: the deals, stages and outcomes live in other modules,
// and the caller's authority sits in the composition layer.
type AssembleFunc func(ctx context.Context, tx pgx.Tx, period Period, scope Scope, asOf time.Time, baseCurrency string) (Assembly, error)

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

// Baseline is a reading as a capture takes it: over the scope's whole
// population, with the fingerprint of who that population was.
type Baseline struct {
	Readings              Readings
	Scope                 Scope
	PopulationFingerprint string
	BaseCurrency          string
	// ForwardMeasure is the measure the landing was built from when read.
	ForwardMeasure ForwardMeasure
	// QualifiedPolicy is the viability rule the qualified readings applied,
	// empty while enforcement is off. Read from the setting, so a period with
	// no deal still records it.
	QualifiedPolicy string
	// Assembly is the whole result the readings came from, kept so a snapshot
	// of the baseline freezes the basis of its chances too.
	Assembly Assembly
}

// BaselineFunc reads the population a call is made about the way a capture
// does. Injected, because the population and its members are resolved in the
// composition layer. A call keeps this reading as its baseline, and the input
// check compares the call with a fresh one.
type BaselineFunc func(ctx context.Context, tx pgx.Tx, scope Scope, period Period, at time.Time) (Baseline, error)

// Handlers is the forecast's HTTP surface.
type Handlers struct {
	store    *Store
	assemble AssembleFunc
	period   PeriodFunc
	resolve  ResolveScopeFunc
	history  HistoryFunc
	measure  MeasureFunc
	holds    HoldsDealsFunc
	baseline BaselineFunc
	// stageChances serves the stage-chances read; see stagechances.go.
	stageChances StageChancesFunc
	// readerRows narrows per-deal rows to what the reader may see one by one.
	readerRows ReaderRowsFunc
	now          func() time.Time
}

// ReaderRowsFunc narrows the per-deal rows of a reading to what the reader may
// see one by one. Injected, because the installation's individual-performance
// rule lives in the composition layer.
type ReaderRowsFunc func(ctx context.Context, tx pgx.Tx, rows []Contribution) ([]Contribution, error)

// WithReaderRows narrows the rows GET /forecast?contributions=true returns.
// Unbound, the door refuses to return rows at all.
func (h Handlers) WithReaderRows(narrow ReaderRowsFunc) Handlers {
	h.readerRows = narrow
	return h
}

// WithCallBaseline keeps a baseline with every call recorded through these
// handlers. Without one a call has no baseline, and its history reads as
// unavailable.
func (h Handlers) WithCallBaseline(baseline BaselineFunc) Handlers {
	h.baseline = baseline
	return h
}

// NewHandlers binds the routes to the store and its seams.
func NewHandlers(
	store *Store, assemble AssembleFunc, period PeriodFunc, resolve ResolveScopeFunc,
	history HistoryFunc, measure MeasureFunc, holds HoldsDealsFunc, now func() time.Time,
) Handlers {
	return Handlers{
		store: store, assemble: assemble, period: period, resolve: resolve,
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
		assembly, err := h.assemble(ctx, tx, period, scope, at, baseCurrency)
		if err != nil {
			return err
		}
		scope, readings := assembly.Scope, assembly.Readings
		out = ReadingsToWire(period, scope, readings, baseCurrency, at)
		out.ScopeLimited = &assembly.Limited
		out.SelfCorrecting = &assembly.SelfCorrecting
		if params.Contributions != nil && *params.Contributions {
			if h.readerRows == nil {
				return errors.New("forecasting: no reader lens is bound, so per-deal rows cannot be narrowed")
			}
			visible, err := h.readerRows(ctx, tx, readings.Contributions)
			if err != nil {
				return err
			}
			rows := ContributionsToWire(visible)
			out.Contributions = &rows
		}
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
		out, err = h.recordWithBaseline(ctx, tx, NewCall{
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

// recordWithBaseline records a call and the reading it was made against. The
// baseline is stamped and read at the server's clock; the period, which the
// caller's as_of chose, rides on the snapshot's own period columns. So a
// caller cannot stamp a capture with a day of their choosing. The reading is
// taken before the write, so the population-sized read runs before the
// previous call is locked. The transaction stays read committed: under repeatable read
// a call waiting on that lock would supersede a call it cannot see, and fork
// the chain.
func (h Handlers) recordWithBaseline(ctx context.Context, tx pgx.Tx, in NewCall) (Call, error) {
	if h.baseline == nil {
		return h.store.RecordCallTx(ctx, tx, in)
	}
	submitted := h.now()
	baseline, err := h.baseline(ctx, tx, in.Scope, in.Period, submitted)
	if err != nil {
		return Call{}, err
	}
	out, err := h.store.RecordCallTx(ctx, tx, in)
	if err != nil {
		return Call{}, err
	}
	snapshot := NewSnapshot{
		Period: in.Period, Scope: baseline.Scope, Trigger: TriggerCall,
		BaseCurrency: baseline.BaseCurrency, Readings: baseline.Readings, TakenAt: submitted,
		CallID: &out.ID, PopulationFingerprint: baseline.PopulationFingerprint,
		ForwardMeasure: baseline.ForwardMeasure, QualifiedPolicyVersion: baseline.QualifiedPolicy,
	}
	snapshot.StampChances(baseline.Assembly)
	if _, err := h.store.TakeSnapshot(ctx, tx, snapshot); err != nil {
		return Call{}, err
	}
	out.HasBaseline = true
	return out, nil
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
