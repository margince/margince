// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The forecast half of the canonical corpus. It covers what a quarter is
// expected to close, why it moved between two snapshots, and whether the inputs
// can be trusted. Each proof calls `read_forecast` by name, as an MCP client
// does, through the composed registry.
//
// The snapshots are written through the forecasting store's own writer, the way
// the nightly job writes them. The input checks come from a real assurance pass.
// Nothing here shapes a row the product would not have made.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// forecastAsker is a team manager who may read the forecast.
func (e *forecastEnv) forecastAsker() context.Context { return e.forecastReader(e.teamManagerCtx()) }

// freezeForecast writes one snapshot of a scope through the forecasting store,
// as the snapshot job does.
func (e *forecastEnv) freezeForecast(
	t *testing.T, at time.Time, scope forecasting.Scope, rows []forecasting.Contribution,
) ids.UUID {
	t.Helper()
	store := forecasting.NewStore(InstallationDB(e.Pool))
	var id ids.UUID
	if err := store.InTx(snapshotWriterCtx(e.WS), func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
		if err != nil {
			return err
		}
		id, err = store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
			Period: period, Scope: scope, Trigger: forecasting.TriggerRecheck, BaseCurrency: base,
			Readings: forecasting.Readings{Contributions: rows}, TakenAt: at,
		})
		return err
	}); err != nil {
		t.Fatalf("freezing the forecast: %v", err)
	}
	return id
}

// frozenContribution is one deal as a snapshot holds it.
func frozenContribution(deal, owner ids.UUID, baseMinor int64, category string) forecasting.Contribution {
	return forecasting.Contribution{
		DealID: deal.String(), Owner: owner.String(), AmountMinor: &baseMinor, Currency: "EUR",
		BaseMinor: &baseMinor, Category: category, StageProbability: 50, WeightedMinor: baseMinor / 2,
		InOpen: true, InEvidence: category == "commit", InBestCase: category != "pipeline",
	}
}

// F01: what the quarter is expected to close, and how much of it is committed.
// A colleague's deal on another team and a deal due in a later year stay out.
func proveF01QuarterForecast(t *testing.T) {
	e := setupForecast(t)
	today, later := new(0), new(500)
	seedMany(3, func() { e.seedDealClosing(t, e.Rep1, "commit", today) })
	seedMany(2, func() { e.seedDealClosing(t, e.Rep1, "best_case", today) })
	seedMany(4, func() { e.seedDealClosing(t, e.Rep1, "pipeline", today) })
	seedMany(2, func() { e.seedDealClosing(t, e.Rep3, "commit", today) })
	seedMany(1, func() { e.seedDealClosing(t, e.Rep1, "commit", later) })

	var got struct {
		OpenMinor     int64  `json:"open_minor"`
		WeightedMinor int64  `json:"weighted_minor"`
		BestCaseMinor int64  `json:"best_case_minor"`
		EvidenceMinor int64  `json:"evidence_minor"`
		Eligible      int    `json:"eligible_count"`
		ScopeKind     string `json:"scope_kind"`
	}
	invokeAnswer(e.forecastAsker(), t, e.toolRegistry(), "read_forecast", `{"view":"readings"}`, &got)
	if got.OpenMinor != 900_000 || got.Eligible != 9 {
		t.Errorf("open %d over %d deals, want 900000 over 9. Another team's deal or a later year's leaked in",
			got.OpenMinor, got.Eligible)
	}
	if got.WeightedMinor != 540_000 {
		t.Errorf("weighted %d, want 540000: the open value at its stage probability", got.WeightedMinor)
	}
	if got.EvidenceMinor != 300_000 || got.BestCaseMinor != 500_000 {
		t.Errorf("committed %d and best case %d, want 300000 and 500000. The commit deals are 3 of the 5 in best case",
			got.EvidenceMinor, got.BestCaseMinor)
	}
}

// F02: why the forecast moved between two snapshots. A deal that grew, a deal
// that arrived and a deal that left each explain their share of the change.
func proveF02ForecastMovement(t *testing.T) {
	e := setupForecast(t)
	seed := func() ids.UUID {
		return e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency,
				source, captured_by)
			VALUES ($1, 'Deal', $2, $3, $4, 100000, 'EUR', 'manual', 'human:x')`,
			e.pipeline, e.stages[pipelineTestStage], e.Rep1)
	}
	grew, left, arrived := seed(), seed(), seed()
	at := time.Date(2038, 2, 1, 9, 0, 0, 0, time.UTC)
	scope := forecasting.Scope{Kind: forecasting.ScopeOwner, ID: &e.Rep1}
	from := e.freezeForecast(t, at, scope, []forecasting.Contribution{
		frozenContribution(grew, e.Rep1, 100_000, "commit"), frozenContribution(left, e.Rep1, 50_000, "pipeline"),
	})
	to := e.freezeForecast(t, at.Add(7*24*time.Hour), scope, []forecasting.Contribution{
		frozenContribution(grew, e.Rep1, 140_000, "commit"), frozenContribution(arrived, e.Rep1, 70_000, "best_case"),
	})

	var got struct {
		OpeningMinor int64 `json:"opening_minor"`
		ClosingMinor int64 `json:"closing_minor"`
		Buckets      []struct {
			Name        string `json:"name"`
			AmountMinor int64  `json:"amount_minor"`
			DealCount   int    `json:"deal_count"`
		} `json:"buckets"`
	}
	invokeAnswer(e.forecastAsker(), t, e.toolRegistry(), "read_forecast",
		fmt.Sprintf(`{"view":"movement","from":%q,"to":%q}`, from, to), &got)
	if got.OpeningMinor != 150_000 || got.ClosingMinor != 210_000 {
		t.Errorf("movement ran %d to %d, want 150000 to 210000", got.OpeningMinor, got.ClosingMinor)
	}
	byName := map[string]int64{}
	for _, bucket := range got.Buckets {
		if bucket.DealCount != 1 {
			t.Errorf("bucket %s holds %d deals, want 1", bucket.Name, bucket.DealCount)
		}
		byName[bucket.Name] = bucket.AmountMinor
	}
	want := map[string]int64{"amount": 40_000, "new": 70_000, "reopened_or_archived": -50_000}
	for name, amount := range want {
		if byName[name] != amount {
			t.Errorf("bucket %s = %d, want %d. The buckets must add up to the 60000 the forecast moved by",
				name, byName[name], amount)
		}
	}
}

// assuredForecast runs a real assurance pass over a workspace with one healthy
// open deal and one deal the rules fault. It returns the workspace and a seat
// that may read the forecast.
func assuredForecast(t *testing.T) (*assuranceJobEnv, context.Context) {
	t.Helper()
	e := setupAssuranceJob(t)
	if err := e.run(t); err != nil {
		t.Fatalf("the assurance pass: %v", err)
	}
	return e, e.Admin()
}

// F03: whether the numbers behind the forecast can be trusted. The check has
// run over both open deals, and it says it is incomplete while a source is not
// connected, rather than reading as clean.
func proveF03ForecastInputsCanBeTrusted(t *testing.T) {
	e, ctx := assuredForecast(t)
	var got struct {
		Status        string `json:"status"`
		Readiness     string `json:"readiness"`
		EligibleDeals int    `json:"eligible_deals"`
	}
	invokeAnswer(ctx, t, composedRegistryOn(InstallationDB(e.Pool)), "read_forecast", `{"view":"input_summary"}`, &got)
	if got.EligibleDeals != 2 {
		t.Errorf("the check read %d deals, want the 2 open ones", got.EligibleDeals)
	}
	if got.Status != "incomplete" || got.Readiness != "checks_incomplete" {
		t.Errorf("status %q readiness %q, want incomplete and checks_incomplete. A source that is not connected "+
			"must not read as a clean forecast", got.Status, got.Readiness)
	}
}

// F04: which deals in the forecast have problems to fix. Only the faulted deal
// is listed, once for each rule it breaks, with the value at stake.
func proveF04DealsWithInputProblems(t *testing.T) {
	e, ctx := assuredForecast(t)
	var got struct {
		Data []struct {
			Type         string `json:"type"`
			Severity     string `json:"severity"`
			SubjectID    string `json:"subject_id"`
			SubjectKind  string `json:"subject_kind"`
			AffectedHome int64  `json:"affected_minor"`
		} `json:"data"`
	}
	invokeAnswer(ctx, t, composedRegistryOn(InstallationDB(e.Pool)), "read_forecast", `{"view":"input_exceptions"}`, &got)
	severities := map[string]string{}
	subjects := map[string]bool{}
	for _, exception := range got.Data {
		severities[exception.Type] = exception.Severity
		subjects[exception.SubjectID] = true
		if exception.SubjectKind != "deal" || exception.AffectedHome != 3_100_000 {
			t.Errorf("%s is about a %s worth %d, want the 3100000 deal", exception.Type, exception.SubjectKind, exception.AffectedHome)
		}
	}
	want := map[string]string{"close_past": "high", "no_economic_buyer": "high", "no_next_step": "low"}
	if len(severities) != len(want) {
		t.Fatalf("listed %v, want exactly the three rules the faulted deal breaks", severities)
	}
	for kind, severity := range want {
		if severities[kind] != severity {
			t.Errorf("%s is %q, want %q", kind, severities[kind], severity)
		}
	}
	if len(subjects) != 1 {
		t.Errorf("the problems name %d deals, want only the faulted one. The healthy deal must not be listed", len(subjects))
	}
}

// F05: which sources feed the forecast and whether they are connected. A
// mailbox nobody connected is said to be not connected, never left out.
func proveF05ForecastSources(t *testing.T) {
	e, ctx := assuredForecast(t)
	var got struct {
		Sources []struct {
			Source string `json:"source"`
			State  string `json:"state"`
		} `json:"sources"`
	}
	invokeAnswer(ctx, t, composedRegistryOn(InstallationDB(e.Pool)), "read_forecast", `{"view":"input_sources"}`, &got)
	states := map[string]string{}
	for _, source := range got.Sources {
		states[source.Source] = source.State
	}
	if states["mail"] != "not_connected" || states["offers"] != "checked" {
		t.Errorf("sources read %v, want mail not_connected and offers checked", states)
	}
}
