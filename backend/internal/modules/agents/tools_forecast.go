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
		Description: "What a period is expected to close, in four readings. " +
			"`won` counts deals by the day they ACTUALLY closed, not the day they were " +
			"expected to. `evidence` is committed pipeline whose close date somebody " +
			"confirmed; a provisional date stays in `open` and out of `evidence`. " +
			"Read `eligible_count` against `priced_count` before quoting a total: an " +
			"unpriced deal is real pipeline contributing zero money. `fx_missing_count` " +
			"is priced deals no rate could convert — also absent from the totals rather " +
			"than counted as zero. " +
			"Quote `as_of`, `timezone` and `base_currency` with the number: a total placed " +
			"in the reader's own zone is a different total.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getForecast",
		InputSchema: schema(`{"type":"object","properties":{
			"period":{"type":"string","enum":["quarter","month","week"],"description":"The window length. Quarters and months follow the installation's own financial year, which may not start in January; a week runs Monday to Sunday."},
			"as_of":{"type":"string","format":"date","description":"Which period to read, by naming a day inside it. Omit for the current one."},
			"scope_kind":{"type":"string","enum":["workspace","team","owner"],"description":"Whose forecast. Omit for this caller's own default population; a wider one is refused."},
			"scope_id":{"type":"string","format":"uuid","description":"The team or owner, for those scopes. Refused with scope_kind=workspace, which names no subject."}},
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

	// ScopeLimited says deals the caller cannot read were left out. A boolean
	// and never a count: a count of what somebody may not read is itself a
	// statement about how much of it there is.
	ScopeLimited *bool `json:"scope_limited,omitempty"`

	// CurrentCall is the standing call for this period, when somebody has made
	// one. Absent means nobody has, which is a real answer.
	CurrentCall json.RawMessage `json:"current_call,omitempty"`
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
		Description: "The difference between two forecast snapshots, classified into named " +
			"causes. Opening plus every bucket equals closing, exactly — so the buckets " +
			"are a complete account of the change and not a selection from it. " +
			"A deal appears in exactly ONE bucket: one that both slipped and was repriced " +
			"has moved for one reason as far as a reader is concerned, which is that it " +
			"left. " +
			"Two buckets are about the machinery rather than the business, and quoting " +
			"them as sales movement is the mistake this classification exists to prevent. " +
			"`definition` means the two snapshots were computed under different rules, and " +
			"then the WHOLE difference is in that bucket. `model` means a probability the " +
			"product re-scored. " +
			"`reopened_or_archived` carries a deal that left the population entirely — " +
			"archived, or no longer visible to this caller — with its whole prior " +
			"contribution, so no money disappears without a row that says where it went.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
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
		Version: toolVersionV1,
		Description: "What last night's input check found, and how much of the pipeline it " +
			"reached. A forecast is only as good as its inputs, and the failures are " +
			"mundane: a close date that went by, an amount that disagrees with the offer " +
			"that was sent, a deal nobody has heard from in ninety days. " +
			"Read `readiness` before quoting any forecast figure. `checks_incomplete` is " +
			"NOT a worse `needs_review` — one says the pipeline has problems, the other " +
			"says we could not look, and reporting the first when the second is true tells " +
			"somebody their pipeline is sound when nobody read the mailbox. " +
			"`sources` says why: each carries the state the run reached, and only a " +
			"`checked` source has a date. An absent or unread source means the run could " +
			"not confirm anything from it, which is different from finding nothing there. " +
			"`eligible_deals` is how much there was to check — compared against an earlier " +
			"run it shows a pass that covered less of the pipeline.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp:    "getForecastAssurance",
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[ForecastAssuranceResult](),
	}
}

func (t forecastInputChecks) Handle(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
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
}

// ForecastAssuranceSourceResult is one source and the state the run reached.
type ForecastAssuranceSourceResult struct {
	Source string `json:"source"`
	State  string `json:"state"`
	// CheckedThrough is present only for a `checked` source: a date on an
	// unread one would claim coverage that did not happen.
	CheckedThrough string `json:"checked_through,omitempty"`
}

// InputChecksReader answers the open findings this caller may see.
type InputChecksReader func(ctx context.Context) (json.RawMessage, error)

// RegisterInputChecksTool joins list_input_checks to the surface.
func RegisterInputChecksTool(r *Registry, read InputChecksReader) {
	r.Register(listInputChecks{read: read})
}

type listInputChecks struct {
	read InputChecksReader
}

func (t listInputChecks) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "list_input_checks", Title: "What the forecast's inputs still need",
		Version: toolVersionV1,
		Description: "The open findings from the nightly input check, most material first. " +
			"Read them before quoting a forecast figure: a close date that went by, or an " +
			"amount that disagrees with the offer that was sent, makes a total wrong " +
			"without making the arithmetic wrong. " +
			"Scoped to what this caller can open, with no count of what was withheld — a " +
			"count of what somebody may not read is itself a statement about how much " +
			"there is. `affected_minor` absent means the money at stake cannot be said, " +
			"not that nothing is at stake.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp:    "listInputChecks",
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[InputChecksResult](),
	}
}

func (t listInputChecks) Handle(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	noteDerivedContent(ctx)
	return t.read(ctx)
}

// InputChecksResult is what list_input_checks answers with.
type InputChecksResult struct {
	// Data are the open findings this caller can see, most material first.
	Data []InputCheckResult `json:"data"`
}

// InputCheckResult is one finding.
type InputCheckResult struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	SubjectKind string `json:"subject_kind"`
	SubjectID   string `json:"subject_id"`
	Severity    string `json:"severity"`
	// AffectedMinor is the money in question. Absent means it cannot be said,
	// which is not the same as nothing being at stake.
	AffectedMinor *int64 `json:"affected_minor,omitempty"`
	Currency      string `json:"currency,omitempty"`
	// Claim and Observed hold structured values whose keys depend on Type.
	//
	// Raw rather than a map: the key set is the exception TYPE's, not this
	// struct's, and a schema generator has nothing to describe a free-form
	// object with. Passing the stored bytes through also keeps this from
	// becoming a second place that knows what each type stores.
	Claim       json.RawMessage `json:"claim"`
	Observed    json.RawMessage `json:"observed"`
	FirstSeenAt string          `json:"first_seen_at"`
	// LastSeenAt is the most recent run that still found it. Something seen for
	// weeks is a different problem from something that appeared last night.
	LastSeenAt string `json:"last_seen_at"`
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
		Description: "Which connectors the nightly check could read, and how far back each " +
			"reaches. Needs the data_coverage grant, which operators hold and sellers do " +
			"not — a refusal here is a seat boundary, not a missing run. " +
			"Only a `checked` source carries a date. On any other state nothing was read, " +
			"and a quiet week is indistinguishable from a broken connector until somebody " +
			"looks.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp:    "getDataCoverage",
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[DataCoverageResult](),
	}
}

func (t dataCoverage) Handle(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
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
