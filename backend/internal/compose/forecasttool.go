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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	store := newForecastStoreFor(pool)
	return func(ctx context.Context, req agents.ForecastRequest) (json.RawMessage, error) {
		at, err := forecastAsOf(req, now)
		if err != nil {
			return nil, err
		}
		scope := forecastToolScope(req)
		kind, err := forecastPeriodKind(req.Period)
		if err != nil {
			return nil, err
		}

		var out agents.ForecastReadingsResult
		err = store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			period, baseCurrency, err := ForecastPeriodAt(ctx, tx, kind, at)
			if err != nil {
				return err
			}
			assembly, err := ForecastAssemble(ctx, tx, period, scope, at, baseCurrency)
			if err != nil {
				return err
			}
			resolved := assembly.Scope
			// The RESOLVED scope, so an agent is told which population the
			// number covers rather than the blank it asked with.
			out = forecastToolResult(period, resolved, assembly.Readings, baseCurrency, at, assembly.Limited)
			out.SelfCorrecting = &assembly.SelfCorrecting
			if req.Contributions {
				visible, limited, err := ForecastReaderContributions(ctx, tx, assembly.Readings.Contributions)
				if err != nil {
					return err
				}
				out.ContributionsLimited = &limited
				if out.Contributions, err = forecastContributionsToTool(visible); err != nil {
					return err
				}
			}
			called, err := addForecastAssertions(ctx, tx, store, period, resolved, baseCurrency, &out)
			if err != nil {
				return err
			}
			// The endpoint answers managed teams before it projects a landing.
			if resolved.Kind == forecasting.ScopeManagedTeams {
				return nil
			}
			landing, err := forecasting.LandingFor(ctx, tx, ForecastForwardMeasure, assembly.Readings, called)
			if err != nil {
				return err
			}
			out.Landing = forecastLandingToTool(landing)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}
}

// addForecastAssertions adds the snapshots and the standing call, which only a
// seat that may assert about the population sees. called is the call's amount,
// nil when this seat sees none, as the endpoint's landing takes it.
func addForecastAssertions(
	ctx context.Context, tx pgx.Tx, store *forecasting.Store, period forecasting.Period,
	resolved forecasting.Scope, baseCurrency string, out *agents.ForecastReadingsResult,
) (called *int64, err error) {
	mayAssert, err := forecasting.MayAssertAbout(ctx, tx, ForecastWritableScope, resolved)
	if err != nil || !mayAssert {
		return nil, err
	}
	refs, err := store.SnapshotRefsTx(ctx, tx, period, resolved, baseCurrency)
	if err != nil {
		return nil, err
	}
	out.Snapshots = forecastSnapshotRefsToTool(refs)
	call, err := store.CurrentCallTx(ctx, tx, period, resolved)
	switch {
	case err == nil:
		rendered := forecastCallToTool(call)
		if err := addCallReview(ctx, tx, rendered, call); err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(rendered)
		if err != nil {
			return nil, fmt.Errorf("compose: rendering the standing call: %w", err)
		}
		out.CurrentCall = encoded
		return &call.AmountMinor, nil
	case forecasting.IsNoStandingCall(err):
		// Nobody has called this period. A real answer, left absent.
		return called, nil
	default:
		return nil, err
	}
}

// forecastLandingToTool is the endpoint's landing in the tool's shape.
func forecastLandingToTool(in crmcontracts.ForecastLanding) *agents.ForecastLandingResult {
	out := &agents.ForecastLandingResult{
		AmountMinor: in.AmountMinor, Measure: string(in.Measure),
		WonMinor: in.WonMinor, RemainingMinor: in.RemainingMinor, SystemMinor: in.SystemMinor,
	}
	if in.Caveat != nil {
		caveat := string(*in.Caveat)
		out.Caveat = &caveat
	}
	return out
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
	return forecasting.DayNamed(parsed), nil
}

// forecastToolScope shapes the request's two scope keys into one scope. It
// cannot fail: the id was already validated by the surface's decoder, and an
// unknown scope_kind is refused downstream by checkScope, which holds the
// table's own CHECK and names the field.
func forecastToolScope(req agents.ForecastRequest) forecasting.Scope {
	// An agent that named no scope is asking about ITS OWN population, the same
	// as a contact who named none. Starting at the workspace made the omission
	// an explicit request for the installation, which a rep- or team-lens
	// passport is now refused outright — so the default answered nothing.
	var scope forecasting.Scope
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
		CoverageNote:       in.CoverageNote(),
	}
	if scope.ID != nil {
		id := scope.ID.String()
		out.ScopeID = &id
	}
	if in.Range != nil {
		out.RangeLowMinor, out.RangeHighMinor = &in.Range.LowMinor, &in.Range.HighMinor
	}
	if q := in.Qualified; q != nil {
		out.Qualified = &agents.ForecastQualifiedResult{
			EvidenceMinor: q.EvidenceMinor, BestCaseMinor: q.BestCaseMinor, WeightedMinor: q.WeightedMinor,
			ExcludedMinor: q.ExcludedMinor, UnknownMinor: q.UnknownMinor,
			ExcludedCount: q.ExcludedCount, UnknownCount: q.UnknownCount, AmountsPartial: q.AmountsPartial,
			UnsupportedMinor: q.UnsupportedMinor, UnsupportedCount: q.UnsupportedCount,
		}
	}
	return out
}

// forecastSnapshotRefsToTool renders the period's snapshot handles. Empty, never
// nil: null reads as "unknown" to a model, and nothing frozen is a real answer.
func forecastSnapshotRefsToTool(refs []forecasting.SnapshotRef) []agents.ForecastSnapshotRefResult {
	out := make([]agents.ForecastSnapshotRefResult, 0, len(refs))
	for _, ref := range refs {
		out = append(out, agents.ForecastSnapshotRefResult{
			ID: ref.ID.String(), TakenAt: ref.TakenAt.UTC().Format(time.RFC3339), Trigger: ref.Trigger,
		})
	}
	return out
}

// forecastCallToTool renders the standing call for a model. The note rides
// along here, unlike on the event: a reader asking what the forecast is wants
// the reason a contact gave for it, and this answer is not a subscription
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
	out["baseline"] = string(crmcontracts.ForecastCallBaselineUnavailable)
	if call.HasBaseline {
		out["baseline"] = string(crmcontracts.ForecastCallBaselineCaptured)
	}
	return out
}

// addCallReview adds the call's open input-check finding, read through the
// list the review panel reads. So a model is told what a manager is told:
// whether the pipeline moved away from the call since it was made.
func addCallReview(ctx context.Context, tx pgx.Tx, rendered map[string]any, call forecasting.Call) error {
	found, err := assuranceCallExceptions(ctx, tx, nil, &RequestedScope{Kind: call.Scope.Kind, ID: call.Scope.ID})
	if err != nil {
		return err
	}
	for _, f := range found {
		if f.SubjectID == call.ID {
			rendered["review"] = map[string]any{"type": f.Type, "severity": f.Severity, "observed": decodeObserved(f.Observed)}
		}
	}
	return nil
}

// decodeObserved renders a finding's stored slots for the tool, empty when
// they do not decode.
func decodeObserved(raw []byte) map[string]any {
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

// movementToolReader answers forecast_movement, through the same store the
// endpoint reads. One classifier, two transports.
func movementToolReader(pool *pgxpool.Pool) agents.MovementReader {
	store := newForecastStoreFor(pool)
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

// forecastPeriodKind reads the window length a caller asked for.
//
// A closed switch rather than a chain of ifs falling through to quarter: an
// unrecognised word used to answer as a QUARTER, so a caller asking for a week
// would get three months labelled with the word they sent — and the reading
// would be wrong in the one way nothing on the page could show them.
func forecastPeriodKind(asked string) (forecasting.PeriodKind, error) {
	switch asked {
	case "":
		// The documented default. An absent period is a caller who did not
		// choose, which is a different thing from one who chose badly.
		return forecasting.PeriodQuarter, nil
	case string(forecasting.PeriodQuarter):
		return forecasting.PeriodQuarter, nil
	case string(forecasting.PeriodMonth):
		return forecasting.PeriodMonth, nil
	case string(forecasting.PeriodWeek):
		return forecasting.PeriodWeek, nil
	default:
		return "", &values.ParseError{
			Field: "period", Code: "unknown_period",
			Message: "a forecast is read over a quarter, a month or a week",
		}
	}
}
