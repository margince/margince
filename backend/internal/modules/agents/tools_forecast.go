// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The forecast_readings tool: what a period is expected to close, and what the
// figure does not cover.
//
// The readings are computed above the modules (they span deals, stages, fx
// rates and the installation's fiscal settings), so the composition root
// injects the reader here as a function — the tool owns the surface contract,
// never the SQL.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ForecastRequest is one reading's arguments, already validated by this
// surface's own decoder.
//
// Decoded HERE rather than behind the seam because the refusal is this
// surface's to word: decodeArgs names the argument a malformed id sat in, and
// an answer an agent can act on is the difference between a corrected retry and
// the same call sent again.
type ForecastRequest struct {
	Period    string   `json:"period"`
	AsOf      string   `json:"as_of"`
	ScopeKind string   `json:"scope_kind"`
	ScopeID   ids.UUID `json:"scope_id"`
	// Contributions asks for the per-deal rows behind the readings.
	Contributions bool `json:"contributions"`
}

// ForecastReader answers the readings for one period and scope, shaped as the
// contract's own ForecastReadings JSON.
type ForecastReader func(ctx context.Context, req ForecastRequest) (json.RawMessage, error)

// RegisterForecastTool joins forecast_readings to the surface once the reader
// exists — the same conditional registration the other seam-backed tools use.
func RegisterForecastTool(r *Registry, read ForecastReader) {
	r.Register(forecastReadings{read: read})
}

type forecastReadings struct {
	read ForecastReader
}

func (t forecastReadings) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "forecast_readings", Title: "Read the forecast", Version: toolVersionV1,
		Description:   forecastReadingsCopy.render(),
		Instead:       forecastReadingsCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp: "getForecast",
		InputSchema: schema(`{"type":"object","properties":{
			"period":{"type":"string","enum":["quarter","month","week"],"description":"The window length. Quarters and months follow the installation's own financial year, which may not start in January; a week runs Monday to Sunday."},
			"as_of":{"type":"string","format":"date","description":"Which period to read, by naming a day inside it. Omit for the current one."},
			"scope_kind":{"type":"string","enum":["workspace","team","owner"],"description":"Whose forecast. Omit for this caller's own default population; a wider one is refused."},
			"scope_id":{"type":"string","format":"uuid","description":"The team or owner, for those scopes. Refused with scope_kind=workspace, which names no subject."},
			"contributions":{"type":"boolean","description":"Add one row per won or open deal you may read: its typed chance, chance in use, evidence count, risk signals, effect and weighted amount."}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ForecastReadingsResult](),
	}
}

func (t forecastReadings) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var req ForecastRequest
	if err := decodeArgs(in, &req); err != nil {
		return nil, err
	}
	noteDerivedContent(ctx)
	return t.read(ctx, req)
}

// ForecastReadingsResult is what forecast_readings answers with: the contract's
// ForecastReadings, restated here because the tool surface declares its own
// output schema.
//
// The counts are not decoration. EligibleCount against PricedCount is what a
// money total does NOT cover, and a model quoting the total without the gap is
// reporting a partial pipeline as a complete one.
type ForecastReadingsResult struct {
	PeriodStart string  `json:"period_start"`
	PeriodEnd   string  `json:"period_end"`
	ScopeKind   string  `json:"scope_kind"`
	ScopeID     *string `json:"scope_id,omitempty"`

	WonMinor      int64 `json:"won_minor"`
	EvidenceMinor int64 `json:"evidence_minor"`
	BestCaseMinor int64 `json:"best_case_minor"`
	OpenMinor     int64 `json:"open_minor"`
	WeightedMinor int64 `json:"weighted_minor"`

	EligibleCount      int `json:"eligible_count"`
	PricedCount        int `json:"priced_count"`
	ConfirmedDateCount int `json:"confirmed_date_count"`
	FxMissingCount     int `json:"fx_missing_count"`

	// AsOf, Timezone and BaseCurrency are the frame the figures were cut in. A
	// total placed in the reader's own zone is a different total.
	AsOf         string `json:"as_of"`
	Timezone     string `json:"timezone"`
	BaseCurrency string `json:"base_currency"`

	// CoverageNote says what the money above does not cover. Composed by
	// forecasting.Readings and never here, and absent when the readings cover
	// every eligible deal.
	CoverageNote string `json:"coverage_note,omitempty"`

	// ScopeLimited says deals the caller cannot read were left out. A boolean
	// and never a count: a count of what somebody may not read is itself a
	// statement about how much of it there is.
	ScopeLimited *bool `json:"scope_limited,omitempty"`

	// CurrentCall is the standing call for this period, when somebody has made
	// one. Absent means nobody has, which is a real answer.
	CurrentCall json.RawMessage `json:"current_call,omitempty"`

	// Snapshots are this period's frozen states, newest first — the ids
	// forecast_movement takes. Empty is a real answer: nothing was frozen.
	Snapshots []ForecastSnapshotRefResult `json:"snapshots"`

	// Qualified is the readings restricted to deals the customer still
	// supports. Absent while the installation does not enforce viability
	// reviews, which means no qualified reading exists, not that it equals the
	// figures above.
	Qualified *ForecastQualifiedResult `json:"qualified,omitempty"`

	// Landing is where the period finishes, the answer GET /forecast gives. The
	// range below sits beside it. Absent for managed teams, as on the endpoint.
	Landing *ForecastLandingResult `json:"landing,omitempty"`

	// SelfCorrecting says whether chances moved with recorded outcomes.
	SelfCorrecting *bool `json:"self_correcting,omitempty"`
	// RangeLowMinor and RangeHighMinor bound the central 80% of finishes under
	// the chances in use. Neither bounds the weighted figure or the landing.
	RangeLowMinor  *int64 `json:"range_low_minor,omitempty"`
	RangeHighMinor *int64 `json:"range_high_minor,omitempty"`
	// Contributions are the per-deal rows, present only when asked for.
	// ContributionsLimited says they hold only the caller's own deals, while
	// the installation switched off individual performance.
	Contributions        []ForecastContributionResult `json:"contributions,omitempty"`
	ContributionsLimited *bool                        `json:"contributions_limited,omitempty"`
}

// ForecastLandingResult mirrors ForecastLanding on GET /forecast. A manager's
// call has no split, so RemainingMinor is zero there and SystemMinor is set.
type ForecastLandingResult struct {
	AmountMinor    int64   `json:"amount_minor"`
	Measure        string  `json:"measure"`
	WonMinor       int64   `json:"won_minor"`
	RemainingMinor int64   `json:"remaining_minor"`
	SystemMinor    *int64  `json:"system_minor,omitempty"`
	Caveat         *string `json:"caveat,omitempty"`
}

// ForecastSignalResult mirrors one ForecastRiskSignal on GET /forecast.
type ForecastSignalResult struct {
	Code         string  `json:"code"`
	Days         *int    `json:"days,omitempty"`
	Count        *int    `json:"count,omitempty"`
	StageID      *string `json:"stage_id,omitempty"`
	ClosePassed  *bool   `json:"close_passed,omitempty"`
	ClosesInDays *int    `json:"closes_in_days,omitempty"`
}

// ForecastContributionResult mirrors one ForecastContribution on GET /forecast.
type ForecastContributionResult struct {
	DealID             string                 `json:"deal_id"`
	DealName           string                 `json:"deal_name"`
	OwnerID            *string                `json:"owner_id,omitempty"`
	StageID            *string                `json:"stage_id,omitempty"`
	Category           *string                `json:"category,omitempty"`
	BaseMinor          *int64                 `json:"base_minor,omitempty"`
	ExclusionReason    *string                `json:"exclusion_reason,omitempty"`
	WeightedMinor      int64                  `json:"weighted_minor"`
	InWon              bool                   `json:"in_won"`
	InOpen             bool                   `json:"in_open"`
	InEvidence         bool                   `json:"in_evidence"`
	InBestCase         bool                   `json:"in_best_case"`
	TypedProbability   *int                   `json:"typed_probability,omitempty"`
	ChanceInUse        int                    `json:"chance_in_use"`
	EvidenceN          *int                   `json:"evidence_n,omitempty"`
	Signals            []ForecastSignalResult `json:"signals"`
	Effect             *float64               `json:"effect,omitempty"`
	AppliedProbability int                    `json:"applied_probability"`
}

// ForecastQualifiedResult mirrors the qualified readings on GET /forecast.
type ForecastQualifiedResult struct {
	EvidenceMinor int64 `json:"evidence_minor"`
	BestCaseMinor int64 `json:"best_case_minor"`
	WeightedMinor int64 `json:"weighted_minor"`
	ExcludedMinor int64 `json:"excluded_minor"`
	UnknownMinor  int64 `json:"unknown_minor"`
	ExcludedCount int   `json:"excluded_count"`
	UnknownCount  int   `json:"unknown_count"`
	// The part of the excluded money with no review behind it.
	UnsupportedMinor int64 `json:"unsupported_minor"`
	UnsupportedCount int   `json:"unsupported_count"`
	// AmountsPartial says some left-out deal had no amount the reader can see.
	AmountsPartial bool `json:"amounts_partial"`
}

// ForecastSnapshotRefResult is one frozen state a movement can start or end at.
type ForecastSnapshotRefResult struct {
	ID      string `json:"id"`
	TakenAt string `json:"taken_at"`
	Trigger string `json:"trigger"`
}

// MovementRequest is one movement's arguments, validated by this surface's own
// decoder — so a malformed snapshot id is refused by name rather than becoming
// a lookup that finds nothing.
type MovementRequest struct {
	From    ids.UUID `json:"from"`
	To      ids.UUID `json:"to"`
	Reading string   `json:"reading"`
}

// MovementReader answers the classified difference between two snapshots.
type MovementReader func(ctx context.Context, req MovementRequest) (json.RawMessage, error)

// RegisterMovementTool joins forecast_movement to the surface.
func RegisterMovementTool(r *Registry, read MovementReader) {
	r.Register(forecastMovement{read: read})
}

type forecastMovement struct {
	read MovementReader
}

func (t forecastMovement) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "forecast_movement", Title: "What moved the forecast", Version: toolVersionV1,
		Description:   forecastMovementCopy.render(),
		Instead:       forecastMovementCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp: "getForecastMovement",
		InputSchema: schema(`{"type":"object","required":["from","to"],"properties":{
			"from":{"type":"string","format":"uuid","description":"The opening snapshot."},
			"to":{"type":"string","format":"uuid","description":"The closing snapshot."},
			"reading":{"type":"string","enum":["open","weighted","evidence","best_case"],"description":"Which money answer this movement explains. A waterfall is drawn for ONE of them; mixing two adds figures that do not belong in one total."}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ForecastMovementResult](),
	}
}

func (t forecastMovement) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var req MovementRequest
	if err := decodeArgs(in, &req); err != nil {
		return nil, err
	}
	noteDerivedContent(ctx)
	return t.read(ctx, req)
}

// ForecastMovementResult is what forecast_movement answers with.
type ForecastMovementResult struct {
	Reading      string `json:"reading"`
	OpeningMinor int64  `json:"opening_minor"`
	ClosingMinor int64  `json:"closing_minor"`
	// Buckets is a COMPLETE account: opening plus every entry equals closing.
	Buckets []ForecastMovementBucketResult `json:"buckets"`
	Deals   []ForecastMovementDealResult   `json:"deals"`
}

// ForecastMovementBucketResult is one named cause and what it moved.
type ForecastMovementBucketResult struct {
	Name        string `json:"name"`
	AmountMinor int64  `json:"amount_minor"`
	DealCount   int    `json:"deal_count"`
}

// ForecastMovementDealResult is one deal's part in the change, in exactly one
// bucket.
type ForecastMovementDealResult struct {
	DealID      string  `json:"deal_id"`
	Bucket      string  `json:"bucket"`
	AmountMinor int64   `json:"amount_minor"`
	FromMinor   *int64  `json:"from_minor,omitempty"`
	ToMinor     *int64  `json:"to_minor,omitempty"`
	AuditID     *string `json:"audit_id,omitempty"`
	ApprovalID  *string `json:"approval_id,omitempty"`
}

// AssuranceReader answers what last night's input check found.
type AssuranceReader func(ctx context.Context) (json.RawMessage, error)

// RegisterAssuranceTool joins forecast_input_checks to the surface.
func RegisterAssuranceTool(r *Registry, read AssuranceReader) {
	r.Register(forecastInputChecks{read: read})
}

type forecastInputChecks struct {
	read AssuranceReader
}

func (t forecastInputChecks) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "forecast_input_checks", Title: "What the forecast's inputs were checked against",
		Version:       toolVersionV1,
		Description:   forecastInputChecksCopy.render(),
		Instead:       forecastInputChecksCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp:    "getForecastAssurance",
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[ForecastAssuranceResult](),
	}
}

func (t forecastInputChecks) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	noteDerivedContent(ctx)
	return t.read(ctx)
}

// ForecastAssuranceResult is what forecast_input_checks answers with.
type ForecastAssuranceResult struct {
	RunID string `json:"run_id"`
	AsOf  string `json:"as_of"`
	// Status is complete or incomplete. An incomplete run still happened and
	// still recorded what it reached.
	Status string `json:"status"`
	// Readiness is the verdict. checks_incomplete means nobody could look,
	// which is a different answer from the pipeline having problems.
	Readiness string `json:"readiness,omitempty"`
	// EligibleDeals is how much there was to check, counted per deal.
	EligibleDeals   int `json:"eligible_deals"`
	EligibleSignals int `json:"eligible_signals,omitempty"`
	// Sources are the sources the run tried, and how far it reached into each.
	// A source absent from this list is one the run did not attempt — which is
	// different from one it attempted and could not read, and the state field
	// is where that distinction lives.
	Sources []ForecastAssuranceSourceResult `json:"sources"`
	// Freshness says whether the records moved after the run read them. An
	// out_of_date reading is the run's verdict on records that have changed.
	Freshness ForecastAssuranceFreshnessResult `json:"freshness"`
}

// ForecastAssuranceFreshnessResult is what changed after the run read it.
type ForecastAssuranceFreshnessResult struct {
	// State is current, out_of_date, or unknown when no run has recorded what
	// it read since changes were first tracked.
	State            string `json:"state"`
	ChangedDeals     int    `json:"changed_deals"`
	ChangedSignals   int    `json:"changed_signals"`
	WorkspaceChanged bool   `json:"workspace_changed"`
	// ChangedSince is the oldest change the run has not read, absent when none.
	ChangedSince string `json:"changed_since,omitempty"`
}

// ForecastAssuranceSourceResult is one source and the state the run reached.
type ForecastAssuranceSourceResult struct {
	Source string `json:"source"`
	State  string `json:"state"`
	// CheckedThrough is present only for a `checked` source: a date on an
	// unread one would claim coverage that did not happen.
	CheckedThrough string `json:"checked_through,omitempty"`
}

// SourceCoverageReader answers how current the sources behind the numbers are.
type SourceCoverageReader func(ctx context.Context) (json.RawMessage, error)

// RegisterCoverageTool joins data_coverage to the surface.
func RegisterCoverageTool(r *Registry, read SourceCoverageReader) {
	r.Register(dataCoverage{read: read})
}

type dataCoverage struct {
	read SourceCoverageReader
}

func (t dataCoverage) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "data_coverage", Title: "How current the sources are", Version: toolVersionV1,
		Description:   dataCoverageCopy.render(),
		Instead:       dataCoverageCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp:    "getDataCoverage",
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[DataCoverageResult](),
	}
}

func (t dataCoverage) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	noteDerivedContent(ctx)
	return t.read(ctx)
}

// DataCoverageResult is what data_coverage answers with.
type DataCoverageResult struct {
	RunID string `json:"run_id"`
	AsOf  string `json:"as_of"`
	// Sources are the ones the run tried. One absent was not attempted, which
	// is different from one attempted and unreadable — the state says which.
	Sources []ForecastAssuranceSourceResult `json:"sources"`
}
