// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The forecast roll-up (B-E09.10) and the "Explain This Number"
// derivation (B-E09.9) over the real migrated Postgres: weighted +
// unweighted totals reconcile exactly to the seeded constituent deals
// (AC-F1), a multi-stakeholder deal counts once (AC-F2), a drill-through
// sums exactly to the aggregate it explains (AC-R6/AC-X1), and the
// explanation never out-sees the report's row scope.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type forecastEnv struct {
	owner    *pgx.Conn
	Pool     *pgxpool.Pool
	handlers reportHandlers
	WS       ids.UUID
	Rep1     ids.UUID // team1
	Rep3     ids.UUID // team2
	Team1    ids.UUID
	Team2    ids.UUID
	pipeline ids.UUID
	// stages keyed by win_probability, all semantic=open
	stages map[int]ids.UUID
}

func setupForecast(t *testing.T) *forecastEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	// Migrated once per test process; every later test resets the data only, as
	// the rest of this package's suites do through integration.Setup — the
	// discipline backend/gates/integrationmigrateonce_test.go enforces module-wide.
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := &forecastEnv{
		owner: owner, WS: ids.NewV7(),
		Rep1: ids.NewV7(), Rep3: ids.NewV7(), Team1: ids.NewV7(), Team2: ids.NewV7(),
		stages: map[int]ids.UUID{},
	}
	// The installation's identity as settings rows: the forecast's
	// slipped-category dimension resolves its zone from the SETTING now, and
	// this env builds its workspace by raw SQL, so bootstrap never seeded them.
	if _, err := owner.Exec(ctx, `INSERT INTO setting (key, value) VALUES
			('installation.name', '"Forecast"'::jsonb),
			('installation.base_currency', '"EUR"'::jsonb),
			('installation.timezone', '"UTC"'::jsonb),
			-- January, seeded explicitly rather than left absent: the period
			-- buckets read it, and a fixture that relied on the registered
			-- default would assert against a row nothing wrote.
			('installation.fiscal_year_start_month', '1'::jsonb)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatalf("seeding the installation settings: %v", err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.WS); err != nil {
		t.Fatal(err)
	}
	for email, u := range map[string]ids.UUID{"rep1@forecast.test": e.Rep1, "rep3@forecast.test": e.Rep3} {
		if _, err := owner.Exec(ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`, u, email); err != nil {
			t.Fatal(err)
		}
	}
	for _, tm := range []ids.UUID{e.Team1, e.Team2} {
		if _, err := owner.Exec(ctx, `INSERT INTO team (id, name) VALUES ($1, $2)`, tm, tm.String()); err != nil {
			t.Fatal(err)
		}
	}
	for u, tm := range map[ids.UUID]ids.UUID{e.Rep1: e.Team1, e.Rep3: e.Team2} {
		if _, err := owner.Exec(ctx, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, tm, u); err != nil {
			t.Fatal(err)
		}
	}

	e.pipeline = e.seedID(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`)
	for position, probability := range map[int]int{0: 20, 1: 55, 2: 60} {
		e.stages[probability] = e.seedID(t,
			`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, $3, $4, 'open', $5)`,
			e.pipeline, fmt.Sprintf("Stage %d", position), position, probability)
	}

	pool, err := testdb.OwnPool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	e.Pool = pool
	e.handlers = reportHandlers{engine: newReportEngine(pool)}
	return e
}

// seedID writes rows through the owner connection, minting the id and nothing
// else: these suites test READ semantics, and the write shape has its own
// suites. No workspace is bound because none of the tables these fixtures reach
// still carries one (ADR-0091 §8 phase D).
func (e *forecastEnv) seedID(t *testing.T, sql string, args ...any) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), sql, append([]any{id}, args...)...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return id
}

// seedOpenDeal plants one live open deal; amountMinor/category/owner may
// be nil (the honest NULL cases every roll-up must survive). The close
// date is comfortably future so the §11 hygiene exclusion stays out of
// these roll-up suites — the exclusion has its own suite.
func (e *forecastEnv) seedOpenDeal(t *testing.T, name string, probability int, owner *ids.UUID, amountMinor *int64, category *string) ids.UUID {
	t.Helper()
	return e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency, forecast_category, expected_close_date, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $6::bigint IS NULL THEN NULL ELSE 'EUR' END, $7, (now() + interval '30 days')::date, 'manual', 'human:x')`,
		name, e.pipeline, e.stages[probability], owner, amountMinor, category)
}

func (e *forecastEnv) dealReadCtx(userID ids.UUID, teams []ids.UUID, scope principal.RowScope) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + userID.String(), UserID: userID, TeamIDs: teams,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal": {Read: true},
				// The forecast buckets "today" in the installation's zone, and
				// that zone is read behind this object now. 0191
				// grants it to all five seeded roles, so no real caller of a
				// report is without it.
				"installation_settings": {Read: true},
			},
			RowScope: scope,
		},
	})
}

func (e *forecastEnv) Admin() context.Context {
	return e.dealReadCtx(ids.NewV7(), nil, principal.RowScopeAll)
}

type reportResultWire struct {
	Report               string           `json:"report"`
	Columns              []string         `json:"columns"`
	Rows                 []map[string]any `json:"rows"`
	TotalRows            int              `json:"total_rows"`
	ExcludedByPermission *int             `json:"excluded_by_permission"`
	DerivationURL        string           `json:"derivation_url"`
}

type derivationWire struct {
	Report               string           `json:"report"`
	Definition           string           `json:"definition"`
	Columns              []string         `json:"columns"`
	Rows                 []map[string]any `json:"rows"`
	Aggregates           map[string]any   `json:"aggregates"`
	TotalRows            int              `json:"total_rows"`
	ExcludedByPermission *int             `json:"excluded_by_permission"`
}

//craft:ignore naked-any decodeWire is the one JSON unmarshal seam; the wire structs above give it shape
func decodeWire(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, into any) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, wantStatus, rec.Body.String())
	}
	dec := json.NewDecoder(rec.Body)
	dec.UseNumber()
	if err := dec.Decode(into); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
}

// runReport and explainReport are generic over the report key so every
// suite in this package shares one spelling of "POST /reports/{report}" and
// "GET .../derivation" against the real handlers, rather than each report's
// suite growing its own copy that differs only in the key.
func (e *forecastEnv) runReport(ctx context.Context, t *testing.T, report, body string) reportResultWire {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/"+report, strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	e.handlers.RunReport(rec, req, report)
	var result reportResultWire
	decodeWire(t, rec, http.StatusOK, &result)
	return result
}

func (e *forecastEnv) explainReport(ctx context.Context, t *testing.T, report, handleURL string) derivationWire {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, handleURL, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	e.handlers.ExplainReport(rec, req, report, crmcontracts.ExplainReportParams{})
	var result derivationWire
	decodeWire(t, rec, http.StatusOK, &result)
	return result
}

// wireInt reads a JSON-decoded numeric cell exactly (UseNumber keeps
// bigint sums out of float64).
func wireInt(t *testing.T, row map[string]any, key string) int64 {
	t.Helper()
	num, ok := row[key].(json.Number)
	if !ok {
		t.Fatalf("cell %q = %v (%T), want a number", key, row[key], row[key])
	}
	v, err := num.Int64()
	if err != nil {
		t.Fatalf("cell %q = %v: %v", key, num, err)
	}
	return v
}

// weightedMinor mirrors formulas-and-rules §6: round(amount ×
// probability / 100) per deal, half away from zero — the ground truth
// the report must reconcile to.
func weightedMinor(amountMinor, probability int64) int64 {
	return (amountMinor*probability + 50) / 100
}

func int64p(v int64) *int64    { return &v }
func stringp(v string) *string { return &v }

func TestForecastRollupReconcilesToConstituentDeals(t *testing.T) {
	e := setupForecast(t)

	// The constituent open deals — amounts chosen so per-deal rounding
	// is exercised (12341×60% and 54321×55% are not whole after /100).
	type constituent struct {
		amount      *int64
		probability int64
		category    *string
	}
	constituents := []constituent{
		{int64p(100000), 20, stringp("commit")},
		{int64p(12341), 60, stringp("commit")},
		{nil, 60, stringp("commit")}, // no amount: counted, sums untouched
		{int64p(999), 55, stringp("best_case")},
		{int64p(54321), 55, nil}, // no category: the NULL group
	}
	for i, c := range constituents {
		e.seedOpenDeal(t, fmt.Sprintf("Deal %d", i), int(c.probability), nil, c.amount, c.category)
	}
	// Closed and archived deals are not forecast.
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, status, closed_at, source, captured_by)
		VALUES ($1, 'Won already', $2, $3, 'won', now(), 'manual', 'human:x')`, e.pipeline, e.stages[60])
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, archived_at, source, captured_by)
		VALUES ($1, 'Archived', $2, $3, 77777, 'EUR', now(), 'manual', 'human:x')`, e.pipeline, e.stages[60])

	// Grouped by currency as well, which a native money sum requires — a total
	// spanning currencies has no unit. Four rows, not three: the amountless
	// deal carries no currency either, so `commit` splits into its EUR rows and
	// that one. The categories are what this reconciles, so the currency
	// dimension is folded back up below rather than asserted on.
	result := e.runReport(e.Admin(), t, "forecast",
		`{"group_by":["forecast_category","currency"]}`)
	if len(result.Rows) != 4 {
		t.Fatalf("rows = %d (%+v), want commit in two currencies, best_case, and the NULL category",
			len(result.Rows), result.Rows)
	}
	if result.DerivationURL == "" {
		t.Error("result-level derivation_url missing")
	}

	// AC-F1: per-group AND overall, weighted + unweighted totals equal
	// the sum over the seeded constituent deals — zero tolerance.
	wantByCategory := map[string]struct{ deals, unweighted, weighted int64 }{}
	for _, c := range constituents {
		key := ""
		if c.category != nil {
			key = *c.category
		}
		want := wantByCategory[key]
		want.deals++
		if c.amount != nil {
			want.unweighted += *c.amount
			want.weighted += weightedMinor(*c.amount, c.probability)
		}
		wantByCategory[key] = want
	}
	// Folded back to categories before comparing. The currency rows of one
	// category are that category, and summing them is safe HERE for the reason
	// the engine cannot rely on in general: every priced deal seeded is EUR.
	gotByCategory := map[string]struct{ deals, unweighted, weighted int64 }{}
	var gotDeals, gotUnweighted, gotWeighted int64
	for _, row := range result.Rows {
		key := ""
		if s, ok := row["forecast_category"].(string); ok {
			key = s
		}
		if _, known := wantByCategory[key]; !known {
			t.Fatalf("unexpected group %q: %+v", key, row)
		}
		if url, ok := row["derivation_url"].(string); !ok || url == "" {
			t.Errorf("group %q: aggregate row without a derivation_url handle", key)
		}
		got := gotByCategory[key]
		got.deals += wireInt(t, row, "deals")
		got.unweighted += wireIntOrZero(t, row, "unweighted_minor")
		got.weighted += wireIntOrZero(t, row, "weighted_minor")
		gotByCategory[key] = got
	}
	for key, want := range wantByCategory {
		got := gotByCategory[key]
		if got != want {
			t.Errorf("group %q = %+v, want %+v", key, got, want)
		}
		gotDeals += got.deals
		gotUnweighted += got.unweighted
		gotWeighted += got.weighted
	}
	if gotDeals != 5 || gotUnweighted != 100000+12341+999+54321 ||
		gotWeighted != weightedMinor(100000, 20)+weightedMinor(12341, 60)+weightedMinor(999, 55)+weightedMinor(54321, 55) {
		t.Errorf("roll-up total = (%d, %d, %d) deals/unweighted/weighted — does not reconcile to the constituent deals",
			gotDeals, gotUnweighted, gotWeighted)
	}
}

// AC-F2: the roll-up aggregates deals, never deal×stakeholder join rows —
// a deal with two stakeholders counts once in the per-owner grouping.
func TestForecastByOwnerCountsAMultiStakeholderDealOnce(t *testing.T) {
	e := setupForecast(t)
	dealID := e.seedOpenDeal(t, "Two champions", 60, &e.Rep1, int64p(50000), stringp("commit"))
	for _, role := range []string{"champion", "economic_buyer"} {
		personID := e.seedID(t, `INSERT INTO person (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, "Stakeholder "+role)
		e.seedID(t, `INSERT INTO relationship (id, kind, deal_id, person_id, role, source, captured_by)
			VALUES ($1, 'deal_stakeholder', $2, $3, $4, 'manual', 'human:x')`, dealID, personID, role)
	}

	result := e.runReport(e.Admin(), t, "forecast", `{"group_by":["owner_id","currency"]}`)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %+v, want exactly one owner group", result.Rows)
	}
	row := result.Rows[0]
	if row["owner_id"] != e.Rep1.String() {
		t.Fatalf("owner_id = %v, want %s", row["owner_id"], e.Rep1)
	}
	if got := wireInt(t, row, "deals"); got != 1 {
		t.Errorf("deals = %d, want 1 — the stakeholder join must not multiply the deal", got)
	}
	if got := wireInt(t, row, "unweighted_minor"); got != 50000 {
		t.Errorf("unweighted = %d, want 50000", got)
	}
	if got := wireInt(t, row, "weighted_minor"); got != 30000 {
		t.Errorf("weighted = %d, want 30000", got)
	}
}

// AC-R6 + AC-X1: resolving an aggregate row's derivation_url returns a
// plain-language definition and source rows that sum EXACTLY to the
// displayed aggregate; each source row carries the weighted value next
// to its base inputs, so the lineage bottoms out with no opaque step.
func TestForecastDerivationDrillThroughReconcilesExactly(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Alpha", 20, &e.Rep1, int64p(100000), stringp("commit"))
	e.seedOpenDeal(t, "Beta", 60, &e.Rep1, int64p(12341), stringp("best_case"))
	e.seedOpenDeal(t, "Gamma", 55, &e.Rep1, nil, stringp("commit"))
	e.seedOpenDeal(t, "Foreign owner", 60, &e.Rep3, int64p(999999), stringp("commit"))

	result := e.runReport(e.Admin(), t, "forecast", `{"group_by":["owner_id","currency"]}`)
	var row map[string]any
	// The owner's EUR row. The plan groups by currency — a native money sum
	// requires it — so rep1 draws one row per currency, and the amountless deal
	// carries none. Taking whichever came last picked that one: a drill-through
	// into a group of one, which is a true answer to a different question.
	for _, r := range result.Rows {
		if r["owner_id"] == e.Rep1.String() && r["currency"] == "EUR" {
			row = r
		}
	}
	if row == nil {
		t.Fatalf("no aggregate row for rep1 in EUR: %+v", result.Rows)
	}

	handle, ok := row["derivation_url"].(string)
	if !ok || handle == "" {
		t.Fatalf("aggregate row has no derivation_url: %+v", row)
	}
	derivation := e.explainReport(e.Admin(), t, "forecast", handle)

	for _, phrase := range []string{
		"open, unarchived deals",
		`owner_id = "` + e.Rep1.String() + `"`,
		`currency = "EUR"`,
		"the sum of weighted_amount_minor as weighted_minor",
	} {
		if !strings.Contains(derivation.Definition, phrase) {
			t.Errorf("definition %q lacks %q", derivation.Definition, phrase)
		}
	}

	// Two, not rep1's three. The third carries no amount and so no currency, so
	// it belongs to a different group of this same plan — and a drill-through
	// that opened it would be reconciling against a row it was not taken from.
	// What this case holds is that the detail equals the figure above it, which
	// the sums below check; the count is here so a group that quietly changed
	// shape is noticed rather than reconciling to a different two.
	if len(derivation.Rows) != 2 || derivation.TotalRows != 2 {
		t.Fatalf("drill-through = %d rows (total %d), want the 2 priced deals in rep1's EUR group: %+v",
			len(derivation.Rows), derivation.TotalRows, derivation.Rows)
	}
	var unweighted, weighted int64
	for _, source := range derivation.Rows {
		if source["amount_minor"] == nil {
			if source["weighted_amount_minor"] != nil {
				t.Errorf("a NULL-amount deal grew a weighted value: %+v", source)
			}
			continue
		}
		amount := wireInt(t, source, "amount_minor")
		probability := wireInt(t, source, "win_probability")
		rowWeighted := wireInt(t, source, "weighted_amount_minor")
		if rowWeighted != weightedMinor(amount, probability) {
			t.Errorf("source row weighted = %d, want round(%d × %d%%) = %d — the derived input must expose its own lineage",
				rowWeighted, amount, probability, weightedMinor(amount, probability))
		}
		unweighted += amount
		weighted += rowWeighted
	}
	if unweighted != wireInt(t, row, "unweighted_minor") {
		t.Errorf("drill-through unweighted sum %d != displayed %d", unweighted, wireInt(t, row, "unweighted_minor"))
	}
	if weighted != wireInt(t, row, "weighted_minor") {
		t.Errorf("drill-through weighted sum %d != displayed %d", weighted, wireInt(t, row, "weighted_minor"))
	}
	// The server-side recompute over the same predicate set must agree too.
	for _, key := range []string{"deals", "unweighted_minor", "weighted_minor"} {
		if got, want := wireInt(t, derivation.Aggregates, key), wireInt(t, row, key); got != want {
			t.Errorf("recomputed aggregate %q = %d != displayed %d", key, got, want)
		}
	}
}

// Deals are readable by every seat that holds the deal grant, whatever its
// own/team/all tier, and the explanation rides the SAME read model as the
// report: a team-scoped rep's forecast groups every owner's deals, the
// drill-through returns them all, a handle pinned to the other rep's owner
// resolves to that rep's deals, and the admin sees exactly the same set.
func TestForecastDerivationReadsEveryDealWhateverTheTier(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Mine A", 20, &e.Rep1, int64p(10000), stringp("commit"))
	e.seedOpenDeal(t, "Mine B", 60, &e.Rep1, int64p(20000), stringp("commit"))
	e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, int64p(40000), stringp("commit"))

	rep := e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam)
	result := e.runReport(rep, t, "forecast", `{"group_by":["owner_id","currency"]}`)
	if len(result.Rows) != 2 {
		t.Fatalf("team-scoped report rows = %+v, want both owners' groups", result.Rows)
	}

	derivation := e.explainReport(rep, t, "forecast", result.DerivationURL)
	if derivation.TotalRows != 3 || len(derivation.Rows) != 3 {
		t.Fatalf("team-scoped drill-through = %d rows (total %d), want all 3 deals",
			len(derivation.Rows), derivation.TotalRows)
	}
	var sum int64
	for _, source := range derivation.Rows {
		sum += wireInt(t, source, "amount_minor")
	}
	if sum != 70000 {
		t.Errorf("team-scoped drill-through sum = %d, want 70000 (every deal, the other rep's included)", sum)
	}

	// A handle pinned to the other rep's owner resolves to that rep's deal
	// under team scope, the same way it does for the admin.
	foreign := e.explainReport(rep, t, "forecast", "/v1/reports/forecast/derivation?by=owner_id&agg=count%3A%3Adeals&owner_id="+e.Rep3.String())
	if foreign.TotalRows != 1 || len(foreign.Rows) != 1 {
		t.Errorf("other-owner drill-through = %d rows (total %d), want that rep's 1 deal", len(foreign.Rows), foreign.TotalRows)
	}

	// Admin (row_scope=all) sees the same three — the tier made no difference.
	full := e.explainReport(e.Admin(), t, "forecast", result.DerivationURL)
	if full.TotalRows != 3 {
		t.Errorf("admin drill-through total = %d, want 3", full.TotalRows)
	}
}

// forecastStatus runs the forecast and reports only the status code, for the
// cases where the interesting outcome is a refusal rather than a result.
func (e *forecastEnv) forecastStatus(ctx context.Context, body string) int {
	return e.reportStatus(ctx, "forecast", body)
}

// reportStatus runs a plan against any report key and yields only the status,
// for the refusals whose whole assertion is which status a caller gets.
func (e *forecastEnv) reportStatus(ctx context.Context, report, body string) int {
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/"+report, strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	e.handlers.RunReport(rec, req, report)
	return rec.Code
}

// The forecast's "today" comes from the installation SETTING's zone.
//
// It asserts a FLIP rather than a message. Asserting on a date would be
// flaky — any two real zones agree about the date for part of every day, so
// the outcome would turn on the hour the suite ran. And the zone never reaches
// the response body: an unresolvable zone is a Postgres fault, which httperr
// masks to an opaque 500 exactly as it should. So the fixture holds everything
// constant and moves only the setting: the same report that answered 200
// stops answering once the SETTING names a zone Postgres cannot resolve, which
// is the proof that the zone is consulted at all.
func TestTheForecastBucketsInTheZoneTheSettingNames(t *testing.T) {
	e := setupForecast(t)
	// A commit deal WITH a close date, because the zone sits inside a CASE
	// that Postgres evaluates per row: over an empty result the expression is
	// never reached, and the fixture would report success in both directions
	// without ever consulting a zone at all.
	commit := "commit"
	amount := int64(100000)
	e.seedOpenDeal(t, "Zoned", 60, nil, &amount, &commit)
	body := `{"group_by":["forecast_category","currency"]}`
	if got := e.forecastStatus(e.Admin(), body); got != http.StatusOK {
		t.Fatalf("the control run answered %d, want 200 — the fixture is broken before the setting moves", got)
	}

	if _, err := e.owner.Exec(context.Background(),
		`UPDATE setting SET value = '"Margince/Nowhere"'::jsonb WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	if got := e.forecastStatus(e.Admin(), body); got == http.StatusOK {
		t.Error("the forecast still answered 200 with an unresolvable zone in the setting; " +
			"the slipped-category dimension never consulted it")
	}
}

// What is a partner bringing us THIS quarter — the pipeline question, asked of
// the same dimension deals-by-stage answers about what already landed.
//
// It was on one of the two deal reports and not the other, so "revenue by
// partner" could be read backwards and never forwards.
func TestForecastGroupsThePipelineByPartner(t *testing.T) {
	e := setupForecast(t)
	northgate := e.seedID(t, `INSERT INTO organization (id, display_name, source, captured_by) VALUES ($1, 'Northgate', 'manual', 'human:x')`)
	kestrel := e.seedID(t, `INSERT INTO organization (id, display_name, source, captured_by) VALUES ($1, 'Kestrel', 'manual', 'human:x')`)
	// Open deals only: the forecast's baseWhere is the pipeline, not history.
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_org_id, partner_attribution, amount_minor, currency, status, source, captured_by)
		VALUES ($1, 'Northgate open one', $2, $3, $4, 'sourced', 30000, 'EUR', 'open', 'manual', 'human:x')`, e.pipeline, e.stages[60], northgate)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_org_id, partner_attribution, amount_minor, currency, status, source, captured_by)
		VALUES ($1, 'Northgate open two', $2, $3, $4, 'influenced', 20000, 'EUR', 'open', 'manual', 'human:x')`, e.pipeline, e.stages[60], northgate)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_org_id, partner_attribution, amount_minor, currency, status, source, captured_by)
		VALUES ($1, 'Kestrel open', $2, $3, $4, 'sourced', 70000, 'EUR', 'open', 'manual', 'human:x')`, e.pipeline, e.stages[60], kestrel)

	result := e.runReport(e.Admin(), t, "forecast",
		`{"group_by":["partner_org_id","currency"],"aggregates":[{"fn":"sum","field":"amount_minor","as":"unweighted_minor"}]}`)

	byPartner := map[string]int64{}
	for _, row := range result.Rows {
		id, ok := row["partner_org_id"].(string)
		if !ok {
			continue
		}
		byPartner[id] = wireInt(t, row, "unweighted_minor")
	}
	if got := byPartner[northgate.String()]; got != 50000 {
		t.Errorf("Northgate open pipeline = %d, want 50000 (both of their open deals)", got)
	}
	if got := byPartner[kestrel.String()]; got != 70000 {
		t.Errorf("Kestrel open pipeline = %d, want 70000", got)
	}
}

// Narrowing the forecast to ONE partner, the dial the deals screen offers.
func TestForecastNarrowsToOnePartner(t *testing.T) {
	e := setupForecast(t)
	wanted := e.seedID(t, `INSERT INTO organization (id, display_name, source, captured_by) VALUES ($1, 'Wanted', 'manual', 'human:x')`)
	other := e.seedID(t, `INSERT INTO organization (id, display_name, source, captured_by) VALUES ($1, 'Other', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_org_id, partner_attribution, amount_minor, currency, status, source, captured_by)
		VALUES ($1, 'Theirs', $2, $3, $4, 'sourced', 40000, 'EUR', 'open', 'manual', 'human:x')`, e.pipeline, e.stages[60], wanted)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_org_id, partner_attribution, amount_minor, currency, status, source, captured_by)
		VALUES ($1, 'Somebody else', $2, $3, $4, 'sourced', 90000, 'EUR', 'open', 'manual', 'human:x')`, e.pipeline, e.stages[60], other)

	result := e.runReport(e.Admin(), t, "forecast",
		fmt.Sprintf(`{"group_by":["partner_org_id","currency"],"aggregates":[{"fn":"sum","field":"amount_minor","as":"unweighted_minor"}],"filters":{"partner_org_id":%q}}`, wanted.String()))

	// One row, and it is theirs. A filter that returned both would read as a
	// working narrow to anyone who only checked the partner they asked for.
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d, want exactly the one partner asked for", len(result.Rows))
	}
	if id, _ := result.Rows[0]["partner_org_id"].(string); id != wanted.String() {
		t.Errorf("row names partner %s, want %s", id, wanted)
	}
	if got := wireInt(t, result.Rows[0], "unweighted_minor"); got != 40000 {
		t.Errorf("filtered total = %d, want 40000", got)
	}
}

// The disclosure guard, the same one deals-by-stage carries: an aggregate must
// not name a partner the caller's own deal read would mask, and an aggregate
// has no per-row place to write "withheld".
func TestForecastByPartnerDoesNotNameAPartnerTheCallerCannotOpen(t *testing.T) {
	e := setupForecast(t)
	hidden := e.seedID(t, `INSERT INTO organization (id, owner_id, display_name, visibility, source, captured_by)
		VALUES ($1, $2, 'Hidden Partners', 'owner', 'manual', 'human:x')`, e.Rep3)
	open := e.seedID(t, `INSERT INTO organization (id, display_name, source, captured_by)
		VALUES ($1, 'Open Partners', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_org_id, partner_attribution, amount_minor, currency, status, source, captured_by)
		VALUES ($1, 'From the hidden partner', $2, $3, $4, 'sourced', 90000, 'EUR', 'open', 'manual', 'human:x')`, e.pipeline, e.stages[60], hidden)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, partner_org_id, partner_attribution, amount_minor, currency, status, source, captured_by)
		VALUES ($1, 'From the open partner', $2, $3, $4, 'sourced', 10000, 'EUR', 'open', 'manual', 'human:x')`, e.pipeline, e.stages[60], open)

	reader := e.dealReadCtx(ids.NewV7(), nil, principal.RowScopeAll)
	result := e.runReport(reader, t, "forecast",
		`{"group_by":["partner_org_id","currency"],"aggregates":[{"fn":"sum","field":"amount_minor","as":"unweighted_minor"}]}`)

	for _, row := range result.Rows {
		if id, ok := row["partner_org_id"].(string); ok && id == hidden.String() {
			t.Errorf("the forecast named partner %s, which this caller's own deal read masks", id)
		}
	}
	// The clause narrows; it does not blank the dimension.
	var sawOpen bool
	for _, row := range result.Rows {
		if id, ok := row["partner_org_id"].(string); ok && id == open.String() {
			sawOpen = true
		}
	}
	if !sawOpen {
		t.Error("the partner this caller CAN open was dropped too — the clause blanked the dimension rather than narrowing it")
	}
}

// Summing money in each row's own currency across a grouping that does not
// split by currency is REFUSED.
//
// Two open deals — 2,500,000 EUR and 1,000,000 USD — grouped by category
// alone answered 3,500,000: euros added to dollars and returned as a plain
// integer, where the honest converted total is 3,420,000 EUR. The default
// plans append currency, so the invariant was held by a default rather than by
// a rule, and a caller naming its own group_by walked straight past it — which
// the MCP schema resource invites by listing group_by as a free choice.
func TestAReportRefusesNativeMoneySummedAcrossCurrencies(t *testing.T) {
	e := setupForecast(t)
	eur := int64(2_500_000)
	usd := int64(1_000_000)
	e.seedOpenDeal(t, "Home", 20, &e.Rep1, &eur, nil)
	e.seedOpenDeal(t, "Abroad", 20, &e.Rep1, &usd, nil)

	status, body := e.runReportStatus(e.Admin(), t, "forecast",
		`{"group_by":["forecast_category"],`+
			`"aggregates":[{"fn":"sum","field":"amount_minor","as":"bad"}]}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("summing amount_minor without a currency grouping answered %d, want 422 — "+
			"the answer is a number with no unit, and it does not look like one", status)
	}
	if !strings.Contains(body, "currency") {
		t.Errorf("the refusal does not name `currency`: %s\nA caller who cannot see what "+
			"to add has been told no and nothing else", body)
	}

	// The same sum WITH the currency split is answered, so this refuses the
	// combination rather than the measure — a report that could no longer
	// report its own money would be a worse fix than the defect.
	ok, _ := e.runReportStatus(e.Admin(), t, "forecast",
		`{"group_by":["forecast_category","currency"],`+
			`"aggregates":[{"fn":"sum","field":"amount_minor","as":"native"}]}`)
	if ok != http.StatusOK {
		t.Errorf("the same sum grouped by currency answered %d, want 200", ok)
	}
}

// runReportStatus is runReport for a case about the ANSWER's status rather than
// its rows: it returns the code and the raw body instead of insisting on 200.
func (e *forecastEnv) runReportStatus(
	ctx context.Context, t *testing.T, report, body string,
) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/"+report, strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	e.handlers.RunReport(rec, req, report)
	return rec.Code, rec.Body.String()
}

// wireIntOrZero is wireInt for a column a GROUP may legitimately have no value
// in: a sum over rows that carry no amount is SQL NULL, not zero, and the two
// mean different things — nothing was measured, rather than measured as none.
// Callers folding groups together read it as zero because that is what adding
// nothing to a total does.
func wireIntOrZero(t *testing.T, row map[string]any, key string) int64 {
	t.Helper()
	if row[key] == nil {
		return 0
	}
	return wireInt(t, row, key)
}
