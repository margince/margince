// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The deals-by-stage report's weighted figure (AC-F1). The per-stage total is
// the sum of each deal's own rounded weighted value. It is not the stage's
// probability applied once to the summed raw total. The reports screen reads a server aggregate with
// no per-deal rows to round, so the engine computes it the AC-F1 way.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDealsByStageWeightedReconcilesToPerDealRounding(t *testing.T) {
	e := setupForecast(t)

	// Two equal deals whose OWN weighted values each round UP (12341×60% =
	// 7404.6 → 7405), while their combined raw total rounds DOWN
	// (24682×60% = 14809.2 → 14809): round(Σamount×p/100) and
	// Σround(amount×p/100) disagree by exactly 1 for this stage.
	const dealAmount = int64(12341)
	e.seedOpenDeal(t, "Alpha", 60, nil, new(dealAmount))
	e.seedOpenDeal(t, "Beta", 60, nil, new(dealAmount))
	// A different stage's deal must not fold into the group under test.
	e.seedOpenDeal(t, "Elsewhere", 20, nil, new(int64(999999)))

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"},{"fn":"sum","field":"weighted_amount_minor","as":"weighted_minor"}]}`)
	row := dealsByStageRow(t, result, e.stages[60].String())

	wantWeighted := weightedMinor(dealAmount, 60) + weightedMinor(dealAmount, 60)
	sumFirstWeighted := weightedMinor(2*dealAmount, 60) // round(Σamount × p / 100) — the rejected methodology
	if wantWeighted == sumFirstWeighted {
		t.Fatal("fixture is broken: the two methodologies agree, so this test cannot discriminate between them")
	}
	if got := wireInt(t, row, "weighted_minor"); got != wantWeighted {
		if got == sumFirstWeighted {
			t.Fatalf("weighted_minor = %d (rounded the column sum once), want %d (Σround(amount×p/100) — rounded per deal, then summed)", got, wantWeighted)
		}
		t.Fatalf("weighted_minor = %d, want %d", got, wantWeighted)
	}
	if got := wireInt(t, row, "amount_minor_sum"); got != 2*dealAmount {
		t.Errorf("amount_minor_sum = %d, want %d", got, 2*dealAmount)
	}
}

// "Explain This Number" resolves the measure too: its drill-through rows
// reconcile to the cent with the displayed weighted_minor, as AC-F1 asks.
func TestDealsByStageWeightedDerivationReconcilesExactly(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Alpha", 60, nil, new(int64(12341)))
	e.seedOpenDeal(t, "Beta", 60, nil, new(int64(12341)))
	e.seedOpenDeal(t, "Elsewhere", 20, nil, new(int64(999999)))

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"},{"fn":"sum","field":"weighted_amount_minor","as":"weighted_minor"}]}`)
	row := dealsByStageRow(t, result, e.stages[60].String())
	handle, ok := row["derivation_url"].(string)
	if !ok || handle == "" {
		t.Fatalf("aggregate row has no derivation_url: %+v", row)
	}

	derivation := e.explainReport(e.Admin(), t, "deals-by-stage", handle)
	if len(derivation.Rows) != 2 || derivation.TotalRows != 2 {
		t.Fatalf("drill-through = %d rows (total %d), want the stage's 2 deals: %+v",
			len(derivation.Rows), derivation.TotalRows, derivation.Rows)
	}
	var weighted int64
	for _, source := range derivation.Rows {
		amount := wireInt(t, source, "amount_minor")
		probability := wireInt(t, source, "win_probability")
		rowWeighted := wireInt(t, source, "weighted_amount_minor")
		if rowWeighted != weightedMinor(amount, probability) {
			t.Errorf("source row weighted = %d, want round(%d × %d%%) = %d",
				rowWeighted, amount, probability, weightedMinor(amount, probability))
		}
		weighted += rowWeighted
	}
	if weighted != wireInt(t, row, "weighted_minor") {
		t.Errorf("drill-through weighted sum %d != displayed %d", weighted, wireInt(t, row, "weighted_minor"))
	}
}

// The board's column totals measure every deal the reader may see, the set it
// draws as cards and the drill-through opens. A total narrowed to the reader's
// own work printed "1 deal" above eight cards.
func TestDealsByStageMeasuresEveryDealTheReaderMaySee(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Mine", 60, &e.Rep1, new(int64(10000)))
	e.seedOpenDeal(t, "Theirs", 60, &e.Rep3, new(int64(20000)))

	rep := e.dealReadCtx(e.Rep1, nil, principal.RowScopeOwn)
	result := e.runReport(rep, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"},{"fn":"sum","field":"weighted_amount_minor","as":"weighted_minor"}]}`)
	row := dealsByStageRow(t, result, e.stages[60].String())
	if got := wireInt(t, row, "deals"); got != 2 {
		t.Fatalf("deals = %d, want both cards the rep's board draws", got)
	}
	if want := weightedMinor(10000, 60) + weightedMinor(20000, 60); wireInt(t, row, "weighted_minor") != want {
		t.Errorf("weighted_minor = %d, want %d (both deals in the stage)",
			wireInt(t, row, "weighted_minor"), want)
	}

	handle, ok := row["derivation_url"].(string)
	if !ok || handle == "" {
		t.Fatalf("aggregate row has no derivation_url: %+v", row)
	}
	derivation := e.explainReport(rep, t, "deals-by-stage", handle)
	if derivation.TotalRows != 2 {
		t.Errorf("drill-through total = %d, want the 2 deals the aggregate counted", derivation.TotalRows)
	}
}

// The board's owner dial is an owner filter: naming a colleague is refused for
// a rep, as on every install-wide report.
func TestDealsByStageRefusesARepsOwnerDialOnAColleague(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Theirs", 60, &e.Rep3, new(int64(20000)))

	rep := e.dealReadCtx(e.Rep1, nil, principal.RowScopeOwn)
	status, body := e.runReportStatus(rep, t, "deals-by-stage",
		fmt.Sprintf(`{"group_by":["stage_id","currency"],"filters":{"owner_id":%q}}`, e.Rep3.String()))
	if status != http.StatusForbidden || !strings.Contains(body, `"code":"owner_not_measurable"`) {
		t.Fatalf("a rep's owner dial on a colleague got %d %s, want 403 owner_not_measurable", status, body)
	}
}

// An unowned deal counts toward a rep's own population as it does in their
// deal list. `owner_id = $me` never matches NULL, so without the unowned arm
// the deal would vanish from the rep's board while it sits on their worklist.
func TestDealsByStageCountsAnUnownedDealForARep(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Unowned", 60, nil, new(int64(10000)))
	e.seedOpenDeal(t, "Mine", 60, &e.Rep1, new(int64(5000)))

	rep := e.dealReadCtx(e.Rep1, nil, principal.RowScopeOwn)
	result := e.runReport(rep, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	row := dealsByStageRow(t, result, e.stages[60].String())
	if got := wireInt(t, row, "deals"); got != 2 {
		t.Fatalf("deals = %d, want 2 (the rep's own deal AND the unowned one) — "+
			"an unowned deal must not silently drop out of a rep's own population", got)
	}
}

// The same unowned deal must count toward a TEAM manager's default
// (ScopeKindManagedTeams) population too, for the identical reason.
func TestDealsByStageCountsAnUnownedDealForATeamManager(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Unowned", 60, nil, new(int64(10000)))

	manager := e.dealReadCtx(ids.NewV7(), []ids.UUID{e.Team1}, principal.RowScopeTeam)
	result := e.runReport(manager, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	row := dealsByStageRow(t, result, e.stages[60].String())
	if got := wireInt(t, row, "deals"); got != 1 {
		t.Fatalf("deals = %d, want 1 (the unowned deal) — a team manager's "+
			"managed-teams population must also admit an unowned row", got)
	}
}

// A deal owned by a seat who shares none of the caller's teams is on the
// manager's board, so it is in the board's totals too.
func TestDealsByStageCountsAnUnrelatedSeatsDealForATeamManager(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Theirs", 60, &e.Rep3, new(int64(20000)))

	manager := e.dealReadCtx(ids.NewV7(), []ids.UUID{e.Team1}, principal.RowScopeTeam)
	result := e.runReport(manager, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	row := dealsByStageRow(t, result, e.stages[60].String())
	if got := wireInt(t, row, "deals"); got != 1 {
		t.Fatalf("deals = %d, want Rep3's Team2 deal — it is a card on this manager's board", got)
	}
}

// The board's column totals split a stage's total by currency, so a
// mixed-currency column can decline to sum.
func TestDealsByStageGroupsByCurrencySeparately(t *testing.T) {
	e := setupForecast(t)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'EUR deal', $2, $3, 100000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60])
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'USD deal', $2, $3, 50000, 'USD', 'manual', 'human:x')`, e.pipeline, e.stages[60])

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)
	var rows []map[string]any
	for _, row := range result.Rows {
		if row["stage_id"] == e.stages[60].String() {
			rows = append(rows, row)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("rows for the stage = %d, want 2 (one per currency): %+v", len(rows), rows)
	}
	sums := map[string]int64{}
	for _, row := range rows {
		currency, ok := row["currency"].(string)
		if !ok {
			t.Fatalf("row %+v: currency cell is not a string", row)
		}
		sums[currency] = wireInt(t, row, "amount_minor_sum")
	}
	if sums["EUR"] != 100000 || sums["USD"] != 50000 {
		t.Errorf("currency-grouped sums = %+v, want EUR=100000 USD=50000", sums)
	}
}

func TestDealsByStageFiltersByCompanyID(t *testing.T) {
	e := setupForecast(t)
	companyA := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'A', 'manual', 'human:x')`)
	companyB := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'B', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Deal A', $2, $3, $4, 10000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], companyA)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Deal B', $2, $3, $4, 20000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], companyB)

	result := e.runReport(e.Admin(), t, "deals-by-stage", fmt.Sprintf(
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}],"filters":{"company_id":%q}}`,
		companyA.String()))
	row := dealsByStageRow(t, result, e.stages[60].String())
	if got := wireInt(t, row, "deals"); got != 1 {
		t.Fatalf("deals = %d, want 1 (only company A's deal)", got)
	}
	if got := wireInt(t, row, "amount_minor_sum"); got != 10000 {
		t.Errorf("amount_minor_sum = %d, want 10000", got)
	}
}

// The question the partner program is actually run on: what has each partner
// brought us. It was unanswerable while partner_sourced was a filter alone —
// the report could say "these deals came from some partner" and never say which.
func TestDealsByStageGroupsRevenueByPartner(t *testing.T) {
	e := setupForecast(t)
	northgate := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Northgate', 'manual', 'human:x')`)
	kestrel := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Kestrel', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Northgate one', $2, $3, $4, 'sourced', 30000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], northgate)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Northgate two', $2, $3, $4, 'influenced', 20000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], northgate)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Kestrel one', $2, $3, $4, 'sourced', 70000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], kestrel)

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["partner_company_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}],"filters":{"partner_sourced":true}}`)

	byPartner := map[string]int64{}
	for _, row := range result.Rows {
		id, ok := row["partner_company_id"].(string)
		if !ok {
			continue
		}
		byPartner[id] = wireInt(t, row, "amount_minor_sum")
	}
	if got := byPartner[northgate.String()]; got != 50000 {
		t.Errorf("Northgate total = %d, want 50000 (both of their deals)", got)
	}
	if got := byPartner[kestrel.String()]; got != 70000 {
		t.Errorf("Kestrel total = %d, want 70000", got)
	}
}

// Grouping by partner must not report a partner the caller could not open.
// A deal read masks partner_company_id when the company is out of reach, and
// without the reference-scope clause an aggregate would hand back that id.
func TestGroupingByPartnerDoesNotNameAPartnerTheCallerCannotOpen(t *testing.T) {
	e := setupForecast(t)
	// Capture-private to Rep3: readable to its owner, invisible to every other
	// seat, exactly as a connector-captured company can be.
	hidden := e.seedID(t, `INSERT INTO company (id, owner_id, display_name, visibility, source, captured_by)
		VALUES ($1, $2, 'Hidden Partners', 'owner', 'manual', 'human:x')`, e.Rep3)
	open := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Open Partners', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, source, captured_by)
		VALUES ($1, 'From the hidden partner', $2, $3, $4, 'sourced', 90000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], hidden)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, source, captured_by)
		VALUES ($1, 'From the open partner', $2, $3, $4, 'sourced', 10000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], open)

	// A reader of every deal who holds no company grant at all.
	reader := e.dealReadCtx(ids.NewV7(), nil, principal.RowScopeAll)
	result := e.runReport(reader, t, "deals-by-stage",
		`{"group_by":["partner_company_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}],"filters":{"partner_sourced":true}}`)

	for _, row := range result.Rows {
		if id, ok := row["partner_company_id"].(string); ok && id == hidden.String() {
			t.Errorf("the report named partner %s, which this caller's own deal read masks", id)
		}
	}
	// The partner they CAN open is still reported: the clause narrows, it does
	// not blank the whole dimension.
	var sawOpen bool
	for _, row := range result.Rows {
		if id, ok := row["partner_company_id"].(string); ok && id == open.String() {
			sawOpen = true
			if got := wireInt(t, row, "amount_minor_sum"); got != 10000 {
				t.Errorf("open partner total = %d, want 10000", got)
			}
		}
	}
	if !sawOpen {
		t.Error("the readable partner vanished too; the clause excluded more than it should")
	}
}

func TestDealsByStageFiltersByPartnerSourced(t *testing.T) {
	e := setupForecast(t)
	partner := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Partner', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Sourced', $2, $3, $4, 'sourced', 10000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], partner)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Direct', $2, $3, 20000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60])

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}],"filters":{"partner_sourced":true}}`)
	row := dealsByStageRow(t, result, e.stages[60].String())
	if got := wireInt(t, row, "deals"); got != 1 {
		t.Fatalf("deals = %d, want 1 (only the partner-sourced deal)", got)
	}
	if got := wireInt(t, row, "amount_minor_sum"); got != 10000 {
		t.Errorf("amount_minor_sum = %d, want 10000", got)
	}
}

// deals-by-stage joins stage, which has its own created_at, so the stalled
// predicate must arrive alias-qualified or the query fails as ambiguous SQL.
func TestDealsByStageStalledFilterWorksUnderTheStageJoin(t *testing.T) {
	e := setupForecast(t)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, source, captured_by, created_at)
		VALUES ($1, 'Idle', $2, $3, 10000, 'EUR', 'manual', 'human:x', now() - interval '90 days')`, e.pipeline, e.stages[60])
	e.seedOpenDeal(t, "Fresh", 60, nil, new(int64(20000)))

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}],"filters":{"stalled":true}}`)
	row := dealsByStageRow(t, result, e.stages[60].String())
	if got := wireInt(t, row, "deals"); got != 1 {
		t.Fatalf("deals = %d, want 1 (only the idle deal)", got)
	}
	if got := wireInt(t, row, "amount_minor_sum"); got != 10000 {
		t.Errorf("amount_minor_sum = %d, want 10000", got)
	}
}

// deals.IsStalled and deals.StalledSQL spell one rule and must agree. The
// margins are tens of days because the SQL side reads the live now(); the
// boundary-exact case is formulas_test.go's job, against a fixed clock.
func TestDealsByStageStalledFilterAgreesWithIsStalled(t *testing.T) {
	e := setupForecast(t)
	now := time.Now().UTC()
	days := func(n int) time.Time { return now.AddDate(0, 0, n) }

	cases := []struct {
		name    string
		created time.Time
		lastAct *time.Time
		wait    *time.Time
	}{
		{"fresh", days(-5), new(days(-2)), nil},
		{"idle past threshold", days(-90), new(days(-70)), nil},
		{"active wait suppresses", days(-90), new(days(-80)), new(days(10))},
		{"expired wait un-suppresses", days(-90), new(days(-80)), new(days(-5))},
	}
	for _, c := range cases {
		e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, created_at, last_activity_at, wait_until, source, captured_by)
			VALUES ($1, $2, $3, $4, 1000, 'EUR', $5, $6, $7, 'manual', 'human:x')`,
			c.name, e.pipeline, e.stages[60], c.created, c.lastAct, c.wait)
	}

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["stage_id"],"aggregates":[{"fn":"count","as":"deals"}],"filters":{"stalled":true}}`)
	row := dealsByStageRow(t, result, e.stages[60].String())

	var wantStalled int64
	for _, c := range cases {
		if deals.IsStalled("open", c.created, c.lastAct, c.wait, false, now) {
			wantStalled++
		}
	}
	if wantStalled == 0 || wantStalled == int64(len(cases)) {
		t.Fatalf("fixture is broken: IsStalled must split these %d cases, not agree on all of them (got %d stalled)", len(cases), wantStalled)
	}
	if got := wireInt(t, row, "deals"); got != wantStalled {
		t.Errorf("SQL filter matched %d deals, Go's IsStalled agrees on %d — the two spellings of §8 have drifted", got, wantStalled)
	}
}

// dealsByStageRow picks one stage's row out of a group-by-stage_id result,
// since the report takes no stage_id filter.
func dealsByStageRow(t *testing.T, result reportResultWire, stageID string) map[string]any {
	t.Helper()
	for _, row := range result.Rows {
		if row["stage_id"] == stageID {
			return row
		}
	}
	t.Fatalf("no row for stage_id %q in %+v", stageID, result.Rows)
	return nil
}

// The board reads its totals from this report with the screen's dials, so
// the report accepts every dial. A refused one answers 422, and the board
// would count its loaded cards as though they were the total.
func TestDealsByStageNarrowsToOnePartner(t *testing.T) {
	e := setupForecast(t)
	wanted := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Wanted', 'manual', 'human:x')`)
	other := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Other', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Theirs', $2, $3, $4, 'sourced', 40000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], wanted)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Somebody else', $2, $3, $4, 'sourced', 90000, 'EUR', 'manual', 'human:x')`, e.pipeline, e.stages[60], other)

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		fmt.Sprintf(`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}],"filters":{"partner_company_id":%q}}`, wanted.String()))

	// One partner's deals only. Both would read as a working narrow to anyone
	// who checked the partner they asked for and stopped.
	var total int64
	for _, row := range result.Rows {
		total += wireInt(t, row, "amount_minor_sum")
	}
	if total != 40000 {
		t.Errorf("total = %d, want 40000 — the filter did not narrow to the partner asked for", total)
	}
}
