// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealsFunc reads the deals in scope for a period, already row-scoped.
//
// Injected rather than imported: the deals live in another module and the row
// scope is applied where the caller's authority already sits, which is the
// composition layer. The boolean says whether anything was withheld — and it is
// a boolean and never a count, because a count of what a caller may not read is
// itself a statement about how much of it there is.
type DealsFunc func(ctx context.Context, tx pgx.Tx, period Period, scope Scope) ([]Deal, bool, error)

// PeriodFunc resolves the installation's window for a day, and the currency its
// money is counted in. Injected for the same reason: the fiscal settings belong
// to identity.
type PeriodFunc func(ctx context.Context, tx pgx.Tx, kind PeriodKind, at time.Time) (Period, string, error)

// Handlers is the forecast's HTTP surface.
type Handlers struct {
	store  *Store
	deals  DealsFunc
	period PeriodFunc
	now    func() time.Time
}

// NewHandlers binds the routes to the store and its two seams.
func NewHandlers(store *Store, deals DealsFunc, period PeriodFunc, now func() time.Time) Handlers {
	return Handlers{store: store, deals: deals, period: period, now: now}
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
		at = params.AsOf.Time
	}
	kind := PeriodQuarter
	if params.Period != nil && *params.Period == crmcontracts.GetForecastParamsPeriodMonth {
		kind = PeriodMonth
	}

	var out crmcontracts.ForecastReadings
	err = h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		period, baseCurrency, err := h.period(ctx, tx, kind, at)
		if err != nil {
			return err
		}
		deals, limited, err := h.deals(ctx, tx, period, scope)
		if err != nil {
			return err
		}
		// The as-of DAY, not the instant. The slipped rule compares calendar
		// days, and handing it a clock makes a deal due today read as slipped
		// from noon onward — which the report engine, comparing dates, would
		// not agree with.
		readings, err := Compute(period, period.LocalDay(at), deals)
		if err != nil {
			return err
		}
		out = readingsToWire(period, scope, readings, baseCurrency, at)
		out.ScopeLimited = &limited

		call, err := h.store.CurrentCallTx(ctx, tx, period, scope)
		switch {
		case err == nil:
			wire := callToWire(call)
			out.CurrentCall = &wire
		case IsNoStandingCall(err):
			// Nobody has called this period. A real answer, and absent rather
			// than an error.
		default:
			return err
		}
		return nil
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
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
		at = body.AsOf.Time
	}
	kind := PeriodQuarter
	if body.Period != nil && *body.Period == crmcontracts.NewForecastCallPeriodMonth {
		kind = PeriodMonth
	}

	var out Call
	err = h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		period, _, err := h.period(ctx, tx, kind, at)
		if err != nil {
			return err
		}
		out, err = h.store.RecordCallTx(ctx, tx, NewCall{
			Period: period, Scope: scope, AmountMinor: body.AmountMinor,
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
	scope := Scope{Kind: ScopeWorkspace}
	if kind != nil {
		scope.Kind = string(*kind)
	}
	if id != nil {
		asID := ids.UUID(*id)
		scope.ID = &asID
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

func readingsToWire(
	period Period, scope Scope, in Readings, baseCurrency string, asOf time.Time,
) crmcontracts.ForecastReadings {
	out := crmcontracts.ForecastReadings{
		PeriodStart:        openapi_types.Date{Time: period.StartDate},
		PeriodEnd:          openapi_types.Date{Time: period.EndDate},
		ScopeKind:          crmcontracts.ForecastReadingsScopeKind(scope.Kind),
		WonMinor:           in.WonMinor,
		EvidenceMinor:      in.EvidenceMinor,
		BestCaseMinor:      in.BestCaseMinor,
		OpenMinor:          in.OpenMinor,
		WeightedMinor:      in.WeightedMinor,
		EligibleCount:      in.EligibleCount,
		PricedCount:        in.PricedCount,
		ConfirmedDateCount: in.ConfirmedDateCount,
		FxMissingCount:     in.FxMissingCount,
		AsOf:               asOf,
		Timezone:           period.Zone.String(),
		BaseCurrency:       baseCurrency,
	}
	if scope.ID != nil {
		id := openapi_types.UUID(*scope.ID)
		out.ScopeId = &id
	}
	return out
}

func callToWire(in Call) crmcontracts.ForecastCall {
	out := crmcontracts.ForecastCall{
		Id:          openapi_types.UUID(in.ID),
		PeriodStart: openapi_types.Date{Time: in.PeriodStart},
		PeriodEnd:   openapi_types.Date{Time: in.PeriodEnd},
		ScopeKind:   crmcontracts.ForecastCallScopeKind(in.Scope.Kind),
		AmountMinor: in.AmountMinor,
		Currency:    in.Currency,
		AuthorId:    openapi_types.UUID(in.AuthorID),
		CreatedAt:   in.CreatedAt,
	}
	if in.Scope.ID != nil {
		id := openapi_types.UUID(*in.Scope.ID)
		out.ScopeId = &id
	}
	if in.Note != "" {
		out.Note = &in.Note
	}
	if in.SupersedesID != nil {
		id := openapi_types.UUID(*in.SupersedesID)
		out.SupersedesId = &id
	}
	return out
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetForecastMovement answers what moved between two snapshots.
func (h Handlers) GetForecastMovement(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetForecastMovementParams,
) {
	reading := ReadingOpen
	if params.Reading != nil {
		reading = Reading(*params.Reading)
	}
	out, err := h.store.Movement(r.Context(), reading,
		ids.UUID(params.From), ids.UUID(params.To))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, movementToWire(reading, out))
}

func movementToWire(reading Reading, in Movement) crmcontracts.ForecastMovement {
	out := crmcontracts.ForecastMovement{
		Reading:      crmcontracts.ForecastMovementReading(reading),
		OpeningMinor: in.OpeningMinor,
		ClosingMinor: in.ClosingMinor,
		// Empty, never nil. "Nothing moved" is a real answer and arrives shaped
		// like the array it is; nil marshals to null, which a reader takes for
		// "unknown".
		Buckets: []crmcontracts.ForecastMovementBucket{},
		Deals:   []crmcontracts.ForecastMovementDeal{},
	}
	for _, b := range in.Buckets {
		out.Buckets = append(out.Buckets, crmcontracts.ForecastMovementBucket{
			Name:        crmcontracts.ForecastMovementBucketName(b.Name),
			AmountMinor: b.AmountMinor,
			DealCount:   b.DealCount,
		})
	}
	for _, d := range in.Deals {
		wire := crmcontracts.ForecastMovementDeal{
			Bucket:      d.Bucket,
			AmountMinor: d.AmountMinor,
			FromMinor:   d.FromMinor,
			ToMinor:     d.ToMinor,
		}
		if parsed, err := ids.Parse(d.DealID); err == nil {
			wire.DealId = openapi_types.UUID(parsed)
		}
		if d.AuditID != nil {
			id := openapi_types.UUID(*d.AuditID)
			wire.AuditId = &id
		}
		if d.ApprovalID != nil {
			id := openapi_types.UUID(*d.ApprovalID)
			wire.ApprovalId = &id
		}
		out.Deals = append(out.Deals, wire)
	}
	return out
}
