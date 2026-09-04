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
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
			deals, resolved, limited, err := ForecastDeals(ctx, tx, period, scope, at, baseCurrency)
			if err != nil {
				return err
			}
			readings, err := forecasting.Compute(period, period.LocalDay(at), deals)
			if err != nil {
				return err
			}
			// The RESOLVED scope, so an agent is told which population the
			// number covers rather than the blank it asked with.
			out = forecastToolResult(period, resolved, readings, baseCurrency, at, limited)
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

// movementToolReader answers forecast_movement, through the same store the
// endpoint reads. One classifier, two transports.
func movementToolReader(pool *pgxpool.Pool) agents.MovementReader {
	store := forecasting.NewStore(InstallationDB(pool))
	return func(ctx context.Context, req agents.MovementRequest) (json.RawMessage, error) {
		reading := forecasting.ReadingOpen
		if req.Reading != "" {
			reading = forecasting.Reading(req.Reading)
		}
		moved, err := store.Movement(ctx, reading, req.From, req.To)
		if err != nil {
			return nil, err
		}
		return json.Marshal(movementToolResult(reading, moved))
	}
}

func movementToolResult(reading forecasting.Reading, in forecasting.Movement) agents.ForecastMovementResult {
	out := agents.ForecastMovementResult{
		Reading:      string(reading),
		OpeningMinor: in.OpeningMinor,
		ClosingMinor: in.ClosingMinor,
		// Empty, never nil: "nothing moved" is a real answer, and null reads as
		// "unknown" to a model.
		Buckets: []agents.ForecastMovementBucketResult{},
		Deals:   []agents.ForecastMovementDealResult{},
	}
	for _, b := range in.Buckets {
		out.Buckets = append(out.Buckets, agents.ForecastMovementBucketResult{
			Name: b.Name, AmountMinor: b.AmountMinor, DealCount: b.DealCount,
		})
	}
	for _, d := range in.Deals {
		deal := agents.ForecastMovementDealResult{
			DealID: d.DealID, Bucket: d.Bucket, AmountMinor: d.AmountMinor,
			FromMinor: d.FromMinor, ToMinor: d.ToMinor,
		}
		if d.AuditID != nil {
			id := d.AuditID.String()
			deal.AuditID = &id
		}
		if d.ApprovalID != nil {
			id := d.ApprovalID.String()
			deal.ApprovalID = &id
		}
		out.Deals = append(out.Deals, deal)
	}
	return out
}

// assuranceToolReader answers forecast_input_checks, through the same store the
// endpoint reads.
func assuranceToolReader(pool *pgxpool.Pool) agents.AssuranceReader {
	store := assurance.NewStore(InstallationDB(pool))
	return func(ctx context.Context) (json.RawMessage, error) {
		run, err := store.LatestRun(ctx)
		if err != nil {
			return nil, err
		}
		coverage, err := store.CoverageFor(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		out := agents.ForecastAssuranceResult{
			RunID:         run.ID.String(),
			AsOf:          run.AsOf.UTC().Format(time.RFC3339),
			Status:        run.Status,
			EligibleDeals: run.EligibleDeals,
			// Empty, never nil: a run that recorded no coverage is a real
			// answer, and null reads as "unknown" to a model.
			Sources: []agents.ForecastAssuranceSourceResult{},
		}
		if run.Readiness != nil {
			out.Readiness = *run.Readiness
		}
		if run.EligibleSignals > 0 {
			out.EligibleSignals = run.EligibleSignals
		}
		for _, c := range coverage {
			source := agents.ForecastAssuranceSourceResult{Source: c.Source, State: c.State}
			// Only a source actually read carries a date.
			if c.State == assurance.CoverageChecked && c.CheckedThrough != nil {
				source.CheckedThrough = c.CheckedThrough.UTC().Format(time.RFC3339)
			}
			out.Sources = append(out.Sources, source)
		}
		return json.Marshal(out)
	}
}

// inputChecksToolReader answers list_input_checks, through the same scoped read
// the endpoint uses.
//
// The scope is what makes this safe to hand a model: the read goes through the
// deal's own visibility, so a finding about a deal the caller cannot open never
// reaches the tool's answer either.
func inputChecksToolReader(pool *pgxpool.Pool) agents.InputChecksReader {
	store := assurance.NewStore(InstallationDB(pool))
	return func(ctx context.Context) (json.RawMessage, error) {
		var found []assurance.Exception
		if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			found, err = AssuranceExceptions(ctx, tx)
			return err
		}); err != nil {
			return nil, err
		}
		// Empty, never nil: "nothing to check" is a real answer, and null reads
		// as "unknown" to a model — which on this surface is the difference
		// between a clean pipeline and an unread one.
		out := agents.InputChecksResult{Data: []agents.InputCheckResult{}}
		for _, e := range found {
			check := agents.InputCheckResult{
				ID: e.ID.String(), Type: e.Type, SubjectKind: e.SubjectKind,
				SubjectID: e.SubjectID.String(), Severity: e.Severity,
				AffectedMinor: e.AffectedMinor, Currency: e.Currency,
				Claim:       storedSlots(e.Claim),
				Observed:    storedSlots(e.Observed),
				FirstSeenAt: e.FirstSeenAt.UTC().Format(time.RFC3339),
				LastSeenAt:  e.LastSeenAt.UTC().Format(time.RFC3339),
			}
			out.Data = append(out.Data, check)
		}
		return json.Marshal(out)
	}
}

// storedSlots passes a stored jsonb object through as it was written.
//
// An absent or malformed value becomes an empty OBJECT rather than null: null
// on this surface reads as "unknown", and a model told the claim is unknown
// would report something different from a check that recorded nothing. One
// malformed row must also not take down a list a model is reading — the row
// still says which deal and which check it is about.
func storedSlots(raw []byte) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(raw)
}

// coverageToolReader answers data_coverage, through the same store the endpoint
// reads. The grant is checked in the store's own read, so a seller reaching
// this tool is refused by the same boundary the screen uses.
func coverageToolReader(pool *pgxpool.Pool) agents.SourceCoverageReader {
	store := assurance.NewStore(InstallationDB(pool))
	return func(ctx context.Context) (json.RawMessage, error) {
		if err := auth.Require(ctx, "data_coverage", principal.ActionRead); err != nil {
			return nil, err
		}
		run, err := store.LatestRun(ctx)
		if err != nil {
			return nil, err
		}
		coverage, err := store.CoverageFor(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		out := agents.DataCoverageResult{
			RunID: run.ID.String(),
			AsOf:  run.AsOf.UTC().Format(time.RFC3339),
			// Empty, never nil: null reads as "unknown" to a model, which here
			// is a different claim from "nothing was tried".
			Sources: []agents.ForecastAssuranceSourceResult{},
		}
		for _, c := range coverage {
			source := agents.ForecastAssuranceSourceResult{Source: c.Source, State: c.State}
			if c.State == assurance.CoverageChecked && c.CheckedThrough != nil {
				source.CheckedThrough = c.CheckedThrough.UTC().Format(time.RFC3339)
			}
			out.Sources = append(out.Sources, source)
		}
		return json.Marshal(out)
	}
}
