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
		Description: "What a period is expected to close, in four readings, plus what the " +
			"figures do not cover. " +
			"`won` counts deals by the day they ACTUALLY closed, never the day they were " +
			"expected to — a deal expected in March and won in April is April's. `evidence` " +
			"is committed pipeline whose close date somebody confirmed; a provisional date " +
			"is a guess, so it is excluded there and still counted in `open`. `weighted` " +
			"applies each deal's stage probability, rounded per deal. " +
			"Read `eligible_count` against `priced_count` before quoting a total: an " +
			"unpriced deal is real pipeline contributing zero money, and the gap is what " +
			"the money readings leave out. `fx_missing_count` is priced deals no rate " +
			"could convert — also absent from the totals rather than counted as zero. " +
			"`scope_limited` true means deals the caller cannot read were left out; there " +
			"is deliberately no count of them. " +
			"Every figure carries the frame it was cut in: `as_of`, the installation's " +
			"timezone, and the base currency. Quote them with the number — a total placed " +
			"in the reader's own zone is a different total.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getForecast",
		InputSchema: schema(`{"type":"object","properties":{
			"period":{"type":"string","enum":["quarter","month"],"description":"The window length. Quarters follow the installation's own financial year, which may not start in January."},
			"as_of":{"type":"string","format":"date","description":"Which period to read, by naming a day inside it. Omit for the current one."},
			"scope_kind":{"type":"string","enum":["workspace","team","owner"],"description":"Whose forecast. Defaults to the whole workspace."},
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
