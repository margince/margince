// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// forecast_readings on the tool surface, answered by the SAME engine the HTTP
// surface reads.
//
// One assembler, two transports. The tool decodes its own arguments — a model
// sends a period name and a scope, not an http.Request — and everything after
// that is the reading the endpoint serves, so the two surfaces cannot come to
// disagree about what a quarter contains or what a total leaves out.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The keys a call is rendered under on the tool surface. Named because the
// linter counts their repetitions across this package and, more usefully,
// because a wire key spelled twice is a wire key that can be spelled two ways.
// fieldCurrency and fieldAmountMinor are the report engine's, in this same
// package — one spelling of a wire key, not a second beside it.
const (
	fieldID           = "id"
	fieldAuthorID     = "author_id"
	fieldCreatedAt    = "created_at"
	fieldNote         = "note"
	fieldSupersedesID = "supersedes_id"
	fieldAsOf         = "as_of"
)

// forecastToolReader answers forecast_readings, through the SAME assembler the
// HTTP surface reads. One engine, two transports: the tool decodes its own
// arguments and everything after that is the reading the endpoint serves, so
// the two cannot come to disagree about what a quarter contains.
//
// The clock is taken here rather than injected: this tool answers "which period
// is it now", and a second clock argument would be a knob nobody turns.
func forecastToolReader(pool *pgxpool.Pool) agents.ForecastReader {
	now := func() time.Time { return time.Now().UTC() }
	store := forecasting.NewStore(InstallationDB(pool))
	return func(ctx context.Context, req agents.ForecastRequest) (json.RawMessage, error) {
		at, err := forecastAsOf(req, now)
		if err != nil {
			return nil, err
		}
		scope := forecastToolScope(req)
		kind := forecasting.PeriodQuarter
		if req.Period == string(forecasting.PeriodMonth) {
			kind = forecasting.PeriodMonth
		}

		var out agents.ForecastReadingsResult
		err = store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			period, baseCurrency, err := ForecastPeriodAt(ctx, tx, kind, at)
			if err != nil {
				return err
			}
			deals, limited, err := ForecastDeals(ctx, tx, period, scope)
			if err != nil {
				return err
			}
			readings, err := forecasting.Compute(period, period.LocalDay(at), deals)
			if err != nil {
				return err
			}
			out = forecastToolResult(period, scope, readings, baseCurrency, at, limited)
			call, err := store.CurrentCallTx(ctx, tx, period, scope)
			switch {
			case err == nil:
				encoded, err := json.Marshal(forecastCallToTool(call))
				if err != nil {
					return fmt.Errorf("compose: rendering the standing call: %w", err)
				}
				out.CurrentCall = encoded
			case forecasting.IsNoStandingCall(err):
				// Nobody has called this period. A real answer, left absent.
			default:
				return err
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}
}

// forecastAsOf reads the day to answer for, refusing a malformed one by name
// rather than silently answering the current period instead — a model that
// asked about last quarter and got this one has no way to tell.
func forecastAsOf(req agents.ForecastRequest, now func() time.Time) (time.Time, error) {
	if req.AsOf == "" {
		return now(), nil
	}
	parsed, err := time.Parse(time.DateOnly, req.AsOf)
	if err != nil {
		return time.Time{}, &values.ParseError{
			Field: fieldAsOf, Code: codeInvalid,
			Message: fieldAsOf + " is a calendar day, as YYYY-MM-DD",
		}
	}
	return parsed, nil
}

// forecastToolScope shapes the request's two scope keys into one scope. It
// cannot fail: the id was already validated by the surface's decoder, and an
// unknown scope_kind is refused downstream by checkScope, which holds the
// table's own CHECK and names the field.
func forecastToolScope(req agents.ForecastRequest) forecasting.Scope {
	scope := forecasting.Scope{Kind: forecasting.ScopeWorkspace}
	if req.ScopeKind != "" {
		scope.Kind = req.ScopeKind
	}
	if !req.ScopeID.IsZero() {
		id := req.ScopeID
		scope.ID = &id
	}
	return scope
}

func forecastToolResult(
	period forecasting.Period, scope forecasting.Scope, in forecasting.Readings,
	baseCurrency string, asOf time.Time, limited bool,
) agents.ForecastReadingsResult {
	out := agents.ForecastReadingsResult{
		PeriodStart:        period.StartDate.Format(time.DateOnly),
		PeriodEnd:          period.EndDate.Format(time.DateOnly),
		ScopeKind:          scope.Kind,
		WonMinor:           in.WonMinor,
		EvidenceMinor:      in.EvidenceMinor,
		BestCaseMinor:      in.BestCaseMinor,
		OpenMinor:          in.OpenMinor,
		WeightedMinor:      in.WeightedMinor,
		EligibleCount:      in.EligibleCount,
		PricedCount:        in.PricedCount,
		ConfirmedDateCount: in.ConfirmedDateCount,
		FxMissingCount:     in.FxMissingCount,
		AsOf:               asOf.UTC().Format(time.RFC3339),
		Timezone:           period.Zone.String(),
		BaseCurrency:       baseCurrency,
		ScopeLimited:       &limited,
	}
	if scope.ID != nil {
		id := scope.ID.String()
		out.ScopeID = &id
	}
	return out
}

// forecastCallToTool renders the standing call for a model. The note rides
// along here, unlike on the event: a reader asking what the forecast is wants
// the reason a person gave for it, and this answer is not a subscription
// somebody acts on unattended.
func forecastCallToTool(call forecasting.Call) map[string]any {
	out := map[string]any{
		fieldID:          call.ID.String(),
		fieldAmountMinor: call.AmountMinor,
		fieldCurrency:    call.Currency,
		fieldAuthorID:    call.AuthorID.String(),
		fieldCreatedAt:   call.CreatedAt.UTC().Format(time.RFC3339),
	}
	if call.Note != "" {
		out[fieldNote] = call.Note
	}
	if call.SupersedesID != nil {
		out[fieldSupersedesID] = call.SupersedesID.String()
	}
	return out
}
