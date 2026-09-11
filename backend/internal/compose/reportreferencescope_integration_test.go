// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A reference scope narrows only what the query NAMES.
//
// A report row can point at a record the reader cannot open — a deal's partner
// company is the case that exists, because a connector mints a captured
// company as `visibility='owner'` and it stays the importing user's until a
// human promotes it. Grouping by that partner would name it, so those rows
// leave the population. Summing by STAGE names no partner at all, and dropping
// the same rows there reports less money than the reader's own deal list shows
// them, for a reason nothing on screen states.
//
// The two halves of one answer have to describe one population: a count and the
// records its drill-through link opens, or the link is not a drill-through.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// hiddenPartnerEnv is the failing scenario from the report that prompted this:
// two open deals in one stage, both readable, one of them naming a partner
// company the reader cannot open.
type hiddenPartnerEnv struct {
	*forecastEnv
	hidden ids.UUID
	open   ids.UUID
}

// seedHiddenPartner stands up that scenario. The amounts differ by an order of
// magnitude so a total can never be read two ways: 90000 is the hidden
// partner's deal and 10000 the open one, and no sum of a subset collides with
// the sum of another.
func seedHiddenPartner(t *testing.T) hiddenPartnerEnv {
	t.Helper()
	e := setupForecast(t)
	// Capture-private to Rep3, exactly as a connector-captured company is until
	// a human promotes it: readable to its owner, invisible to every other seat
	// including an admin's.
	hidden := e.seedID(t, `INSERT INTO company (id, owner_id, display_name, visibility, source, captured_by)
		VALUES ($1, $2, 'Hidden Partners', 'owner', 'manual', 'human:x')`, e.Rep3)
	open := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Open Partners', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, expected_close_date, source, captured_by)
		VALUES ($1, 'From the hidden partner', $2, $3, $4, 'sourced', 90000, 'EUR', (now() + interval '30 days')::date, 'manual', 'human:x')`,
		e.pipeline, e.stages[60], hidden)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_company_id, partner_attribution, amount_minor, currency, expected_close_date, source, captured_by)
		VALUES ($1, 'From the open partner', $2, $3, $4, 'sourced', 10000, 'EUR', (now() + interval '30 days')::date, 'manual', 'human:x')`,
		e.pipeline, e.stages[60], open)
	return hiddenPartnerEnv{forecastEnv: e, hidden: hidden, open: open}
}

// blindReader reads every deal and holds no company grant, so the hidden
// partner is out of reach for them and the open one is not.
func (e hiddenPartnerEnv) blindReader() context.Context {
	return e.dealReadCtx(ids.NewV7(), nil, principal.RowScopeAll)
}

// A stage total counts both deals. The partner is never named by this
// question, so excluding its deal buys no privacy and costs the reader 90000
// of their own pipeline.
func TestAStageTotalKeepsADealWhosePartnerTheReaderCannotOpen(t *testing.T) {
	e := seedHiddenPartner(t)

	result := e.runReport(e.blindReader(), t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)
	row := dealsByStageRow(t, result, e.stages[60].String())

	if got := wireInt(t, row, "deals"); got != 2 {
		t.Errorf("deals = %d, want 2 — the reader may open both, and this question names no partner", got)
	}
	if got := wireInt(t, row, "amount_minor_sum"); got != 100000 {
		t.Errorf("amount_minor_sum = %d, want 100000 (90000 + 10000)", got)
	}
	// The exclusion this replaces was not reported either, which is what made
	// it unreadable: a short count with nothing beside it saying so.
	if result.ExcludedByPermission != nil {
		t.Errorf("excluded_by_permission = %d, want null — nothing was withheld from this total",
			*result.ExcludedByPermission)
	}
}

// The same reader grouping BY partner still never sees the partner they cannot
// open. This is the case the exclusion exists for, and the change must not
// widen it: an aggregate has no per-row place to write "withheld", and folding
// those deals under a null key would still say that some partner brought them.
func TestGroupingByPartnerStillExcludesThePartnerTheReaderCannotOpen(t *testing.T) {
	e := seedHiddenPartner(t)

	result := e.runReport(e.blindReader(), t, "deals-by-stage",
		`{"group_by":["partner_company_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)

	var sawOpen bool
	for _, row := range result.Rows {
		id, ok := row["partner_company_id"].(string)
		if !ok {
			continue
		}
		if id == e.hidden.String() {
			t.Errorf("the report named partner %s, which this caller's own deal read masks", id)
		}
		if id == e.open.String() {
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

// Filtering BY a partner is scoped like grouping by one, even though it prints
// no id. The answer's SHAPE discloses the reference: "how much did this partner
// bring" answered over a partner the caller cannot open confirms which deals
// are theirs.
//
// Both arms, because the refusal alone is green against an engine that answers
// nobody: the readable partner's own filter must still return their deal.
func TestFilteringByAPartnerIsScopedLikeGroupingByOne(t *testing.T) {
	e := seedHiddenPartner(t)
	reader := e.blindReader()

	hidden := e.runReport(reader, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}],`+
			`"filters":{"partner_company_id":"`+e.hidden.String()+`"}}`)
	for _, row := range hidden.Rows {
		if got := wireInt(t, row, "deals"); got != 0 {
			t.Errorf("deals = %d, want 0 — filtering on an unreadable partner must not confirm their deals", got)
		}
	}

	open := e.runReport(reader, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}],`+
			`"filters":{"partner_company_id":"`+e.open.String()+`"}}`)
	if len(open.Rows) == 0 {
		t.Fatal("filtering on the READABLE partner returned nothing; the refusal above would pass against an engine that answers nobody")
	}
	for _, row := range open.Rows {
		if got := wireInt(t, row, "deals"); got != 1 {
			t.Errorf("deals = %d, want 1 for the partner this reader may open", got)
		}
		if got := wireInt(t, row, "amount_minor_sum"); got != 10000 {
			t.Errorf("amount_minor_sum = %d, want 10000", got)
		}
	}
}

// A report answering with its DEFAULT plan is scoped by what that plan groups
// by, not by what the caller happened to type.
//
// open-deals-per-company groups by company_id when the request names no
// group-by, and company_id is a reference. A narrowing derived from the
// request alone reads "this query names nothing", and the answer then keys a
// row on every company in the installation — including one captured into
// a colleague's mailbox and never promoted.
func TestADefaultGroupByIsScopedLikeAnAskedForOne(t *testing.T) {
	e := setupForecast(t)
	hidden := e.seedID(t, `INSERT INTO company (id, owner_id, display_name, visibility, source, captured_by)
		VALUES ($1, $2, 'Hidden Customer', 'owner', 'manual', 'human:x')`, e.Rep3)
	open := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Open Customer', 'manual', 'human:x')`)
	for _, company := range []ids.UUID{hidden, open} {
		e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, amount_minor, currency, expected_close_date, source, captured_by)
			VALUES ($1, 'Deal', $2, $3, $4, 50000, 'EUR', (now() + interval '30 days')::date, 'manual', 'human:x')`,
			e.pipeline, e.stages[60], company)
	}

	// The empty request: no group_by, so the spec's default decides.
	result := e.runReport(e.dealReadCtx(ids.NewV7(), nil, principal.RowScopeAll), t,
		"open-deals-per-company", `{}`)

	var sawOpen bool
	for _, row := range result.Rows {
		id, ok := row["company_id"].(string)
		if !ok {
			continue
		}
		if id == hidden.String() {
			t.Errorf("the default plan named company %s, which this caller cannot open", id)
		}
		if id == open.String() {
			sawOpen = true
		}
	}
	if !sawOpen {
		t.Error("the readable company vanished too; the default plan narrowed more than it should")
	}
}

// The RESULT-level handle explains the same population its headline reported,
// including when that headline grouped by a reference.
//
// A report grouped by partner excludes the partners its reader cannot open, so
// the answer covers fewer deals than the reader's own list. The handle beside
// that answer explains every cell at once and binds no group value — so unless
// it also NAMES the dimension, the drill-through cannot tell this report from
// one grouped by stage, and opens deals the count never counted.
func TestTheResultHandleExplainsThePopulationItsHeadlineReported(t *testing.T) {
	e := seedHiddenPartner(t)
	reader := e.blindReader()

	result := e.runReport(reader, t, "deals-by-stage",
		`{"group_by":["partner_company_id"],"aggregates":[{"fn":"count","as":"deals"}],"filters":{"partner_sourced":true}}`)

	var counted int64
	for _, row := range result.Rows {
		counted += wireInt(t, row, "deals")
	}
	if counted != 1 {
		t.Fatalf("the headline counted %d deals, want 1 — only the open partner's is nameable", counted)
	}

	detail := e.explainReport(reader, t, "deals-by-stage", result.DerivationURL)
	if int64(detail.TotalRows) != counted {
		t.Errorf("the answer counted %d deals and its result handle opens %d; "+
			"a count and the records its link opens are one population",
			counted, detail.TotalRows)
	}
	for _, row := range detail.Rows {
		if id, ok := row["partner_company_id"].(string); ok && id == e.hidden.String() {
			t.Errorf("the drill-through named partner %s, which the headline excluded", id)
		}
	}
}

// The typed analytics grammar scopes a filter on a reference the same way.
//
// Its filter field is resolved against the analytics schema — the spec's
// dimensions and measures — so a narrowing that looked the field up in the
// report engine's own filter map missed wherever the two disagree.
// open-deals-per-company is that spec: company_id is a dimension there and
// not a filter, which made a filtered count answer whether a capture-private
// company exists and how many deals point at it.
func TestAnAnalyticsFilterOnAReferenceIsScoped(t *testing.T) {
	e := setupForecast(t)
	hidden := e.seedID(t, `INSERT INTO company (id, owner_id, display_name, visibility, source, captured_by)
		VALUES ($1, $2, 'Hidden Customer', 'owner', 'manual', 'human:x')`, e.Rep3)
	open := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Open Customer', 'manual', 'human:x')`)
	// Above the privacy floor on both sides, so a refusal can never stand in
	// for the scope and pass this test for the wrong reason.
	for i := 0; i < 6; i++ {
		for _, company := range []ids.UUID{hidden, open} {
			e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, amount_minor, currency, expected_close_date, source, captured_by)
				VALUES ($1, 'Deal', $2, $3, $4, 50000, 'EUR', (now() + interval '30 days')::date, 'manual', 'human:x')`,
				e.pipeline, e.stages[60], company)
		}
	}

	ask := func(company ids.UUID) AnalyticsAnswer {
		t.Helper()
		answer, err := e.askAnalytics(e.reportReaderCtx(), t, analyticsquery.Query{
			Entity:   "open-deals-per-company",
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
			Filters: []analyticsquery.Filter{
				{Field: "company_id", Op: analyticsquery.OpEq, Value: company.String()},
			},
		})
		if err != nil {
			t.Fatalf("analytics query: %v", err)
		}
		return answer
	}

	for _, row := range ask(hidden).Rows {
		if got, ok := row["n"]; ok && got != nil {
			t.Errorf("filtering on a capture-private company answered n = %v; "+
				"a count over an unreadable reference confirms it exists and how many deals name it", got)
		}
	}
	// The readable company still answers, or the arm above proves only
	// that this engine refuses everybody.
	var answered bool
	for _, row := range ask(open).Rows {
		if got, ok := row["n"]; ok && got != nil {
			answered = true
		}
	}
	if !answered {
		t.Error("filtering on the READABLE company answered nothing; the refusal above would be vacuous")
	}
}

// The count and the records its link opens are one population.
//
// This is the half the report that prompted the change could see: the stage
// said 1 and clicking it opened 2. Both numbers are now 2, and they are read
// from the same handle the response minted.
func TestTheDrillThroughOpensExactlyWhatTheStageCounted(t *testing.T) {
	e := seedHiddenPartner(t)
	reader := e.blindReader()

	result := e.runReport(reader, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)
	row := dealsByStageRow(t, result, e.stages[60].String())
	counted := wireInt(t, row, "deals")

	detail := e.explainReport(reader, t, "deals-by-stage", result.DerivationURL)
	if int64(detail.TotalRows) != counted {
		t.Fatalf("the stage counted %d deals and its drill-through opens %d; a count and its link are one population",
			counted, detail.TotalRows)
	}
	if got := wireInt(t, detail.Aggregates, "amount_minor_sum"); got != 100000 {
		t.Errorf("drill-through amount_minor_sum = %d, want 100000 — the recompute must reconcile to the headline", got)
	}
}

// The drill-through returns every dimension, so it hands back the partner id
// even when the headline grouped by stage. The unreadable one comes back NULL
// and its ROW STAYS — which is what an ordinary read of the same deal does
// (deals/fieldmask.go blanks the reference and keeps the deal).
func TestTheDrillThroughBlanksAnUnreadablePartnerAndKeepsItsRow(t *testing.T) {
	e := seedHiddenPartner(t)
	reader := e.blindReader()

	result := e.runReport(reader, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)
	detail := e.explainReport(reader, t, "deals-by-stage", result.DerivationURL)

	if len(detail.Rows) != 2 {
		t.Fatalf("drill-through returned %d rows, want 2 — the reader may open both deals", len(detail.Rows))
	}
	var blanked, named int
	for _, row := range detail.Rows {
		partner, present := row["partner_company_id"]
		if !present {
			t.Fatal("the drill-through dropped the partner_company_id column entirely; it returns every dimension")
		}
		switch id, isString := partner.(string); {
		case partner == nil:
			blanked++
		case isString && id == e.open.String():
			named++
		case isString && id == e.hidden.String():
			t.Errorf("the drill-through named partner %s, which this caller's own deal read masks", id)
		default:
			t.Errorf("unexpected partner_company_id %v (%T)", partner, partner)
		}
	}
	if blanked != 1 {
		t.Errorf("%d rows came back with a blank partner, want exactly 1 (the hidden one)", blanked)
	}
	if named != 1 {
		t.Errorf("%d rows named the readable partner, want exactly 1 — masking must not blank what the reader may open", named)
	}
}

// The owner of the captured company sees their own partner named, in the
// same drill-through that blanks it for everybody else. Without this the suite
// above would pass against an engine that blanked partner_company_id for every
// caller, which is a different product and not a privacy boundary.
func TestTheOwnerOfACapturedPartnerStillSeesItNamed(t *testing.T) {
	e := seedHiddenPartner(t)
	owner := e.dealReadCtx(e.Rep3, nil, principal.RowScopeAll)

	result := e.runReport(owner, t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)
	detail := e.explainReport(owner, t, "deals-by-stage", result.DerivationURL)

	var sawHidden bool
	for _, row := range detail.Rows {
		if id, ok := row["partner_company_id"].(string); ok && id == e.hidden.String() {
			sawHidden = true
		}
	}
	if !sawHidden {
		t.Error("the capturing user's own partner was blanked from them; capture privacy is theirs, not a mask on everyone")
	}
}

// A JOINED ATTRIBUTE inherits the row scope of the id it hangs off.
//
// size_band is a column of the COMPANY, not of the deal, so it reaches the
// query over win-loss's join and its expression is `company.size_band`.
// referenceScopes renders `ref.id = <column>` and a size band is not an id, so
// it matched no entry and the dimension carried no row scope at all: a seat
// excluded from an account could group by that account's size and read its win
// count off the row. spec.scopeVia names the id column the dimension inherits
// from, and this is what proves the clause reaches it.
//
// Both arms, because a refusal that excluded EVERY company would satisfy the
// first assertion and break the report: the readable company's band must still
// carry its own deal.
func TestGroupingByAJoinedAttributeIsScopedLikeTheIdItHangsOff(t *testing.T) {
	e := setupForecast(t)
	// Capture-private to Rep3, exactly as seedHiddenPartner's is: readable to
	// its owner and invisible to every other seat.
	hidden := e.seedID(t, `INSERT INTO company (id, owner_id, display_name, size_band, visibility, source, captured_by)
		VALUES ($1, $2, 'Hidden Enterprise', '1001-5000', 'owner', 'manual', 'human:x')`, e.Rep3)
	open := e.seedID(t, `INSERT INTO company (id, display_name, size_band, source, captured_by)
		VALUES ($1, 'Open Startup', '11-50', 'manual', 'human:x')`)
	// Won deals, because win-loss reads none that are still open. The amounts
	// differ by an order of magnitude so no subset's total collides with
	// another's.
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, status, closed_at, amount_minor, currency, fx_rate_to_base, expected_close_date, source, captured_by)
		VALUES ($1, 'Won at the hidden company', $2, $3, $4, 'won', now() - interval '2 days', 90000, 'EUR', 1.0, (now() - interval '2 days')::date, 'manual', 'human:x')`,
		e.pipeline, e.stages[60], hidden)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, status, closed_at, amount_minor, currency, fx_rate_to_base, expected_close_date, source, captured_by)
		VALUES ($1, 'Won at the open company', $2, $3, $4, 'won', now() - interval '2 days', 10000, 'EUR', 1.0, (now() - interval '2 days')::date, 'manual', 'human:x')`,
		e.pipeline, e.stages[60], open)

	blind := e.dealReadCtx(ids.NewV7(), nil, principal.RowScopeAll)
	result := e.runReport(blind, t, "win-loss",
		`{"group_by":["size_band","currency"],"aggregates":[{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_minor","as":"amount_minor_sum"}]}`)

	var sawOpenBand bool
	for _, row := range result.Rows {
		band, ok := row["size_band"].(string)
		if !ok {
			continue
		}
		if band == "1001-5000" {
			t.Errorf("the report grouped a win under band %q, which belongs to a company "+
				"this caller's own read masks: the joined attribute carried no row scope", band)
		}
		if band == "11-50" {
			sawOpenBand = true
			if got := wireInt(t, row, "amount_minor_sum"); got != 10000 {
				t.Errorf("the readable company's band totals %d, want 10000", got)
			}
		}
	}
	if !sawOpenBand {
		t.Error("the readable company's band vanished too; the clause excluded more than it should")
	}

	// The DRILL-THROUGH half, on the handle shape the product does not mint.
	//
	// A handle carrying `by=` is covered by namedByDerivation's groupBy loop.
	// A hand-built one that pins size_band as a BARE PREDICATE carries it as an
	// EXPRESSION — `company.size_band` — which matches no referenceScopes key, so
	// only the walk-back from spec.scopeVia names the id column and narrows the
	// rows. Without that loop this URL opens the hidden company's deal.
	bare := e.explainReport(blind, t, "win-loss",
		"/v1/reports/win-loss/derivation?agg=count%3A%3Adeals&size_band=1001-5000")
	if bare.TotalRows != 0 {
		t.Errorf("a drill-through pinning the hidden company's band opened %d row(s), want 0: "+
			"the predicate reached the query without activating the company's row scope",
			bare.TotalRows)
	}
}

// The ANALYTICS surface honours scopeVia too.
//
// The typed grammar derives its vocabulary from the same reportSpecs, so
// publishing size_band for win-loss published it on POST /analytics/query and
// the run_analytics_query tool at the same moment. That surface builds its own
// referencedColumns (namedByAnalyticsQuery), and a fix applied to the report
// builder alone leaves the identical leak one surface over: group by size_band
// and the hidden company's win count comes back, or filter on a band and get a
// yes/no oracle for whether a company that size closed anything.
//
// Six deals per company, so the answer clears analyticsquery.DefaultFloor and a
// small-group refusal can never stand in for the row scope and pass this for
// the wrong reason.
func TestTheAnalyticsSurfaceScopesAJoinedAttributeToo(t *testing.T) {
	e := setupForecast(t)
	hidden := e.seedID(t, `INSERT INTO company (id, owner_id, display_name, size_band, visibility, source, captured_by)
		VALUES ($1, $2, 'Hidden Enterprise', '1001-5000', 'owner', 'manual', 'human:x')`, e.Rep3)
	open := e.seedID(t, `INSERT INTO company (id, display_name, size_band, source, captured_by)
		VALUES ($1, 'Open Startup', '11-50', 'manual', 'human:x')`)
	for i := 0; i < 6; i++ {
		for _, company := range []ids.UUID{hidden, open} {
			e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, status, closed_at, amount_minor, currency, fx_rate_to_base, expected_close_date, source, captured_by)
				VALUES ($1, 'Won deal', $2, $3, $4, 'won', now() - interval '2 days', 50000, 'EUR', 1.0, (now() - interval '2 days')::date, 'manual', 'human:x')`,
				e.pipeline, e.stages[60], company)
		}
	}

	grouped, err := e.askAnalytics(e.reportReaderCtx(), t, analyticsquery.Query{
		Entity:   "win-loss",
		GroupBy:  []string{"size_band"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
	})
	if err != nil {
		t.Fatalf("analytics group by size_band: %v", err)
	}

	var sawOpenBand bool
	for _, row := range grouped.Rows {
		band, ok := row["size_band"].(string)
		if !ok {
			continue
		}
		if band == "1001-5000" {
			t.Errorf("the analytics surface grouped %v win(s) under the hidden company's "+
				"band: namedByAnalyticsQuery did not activate the company's row scope", row["n"])
		}
		if band == "11-50" {
			sawOpenBand = true
		}
	}
	// The readable company's band still answers, or the arm above proves only
	// that this surface refuses everybody.
	if !sawOpenBand {
		t.Error("the readable company's band answered nothing; the refusal above would be vacuous")
	}
}
