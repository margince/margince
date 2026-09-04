// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A saved answer meeting a reader who is not the asker.
//
// A report sentence cites a run id and a cell, and the number is dereferenced
// when somebody reads the report. That makes a saved run the one object here
// whose rows were computed for ONE person and are later fetched by ANOTHER, so
// the whole file is about a single question: does the pointer carry data across
// a permission boundary?
//
// It must not. What a saved run fixes is the QUESTION. The answer is recomputed
// under whoever is reading, which is why two people can cite one cell and
// legitimately see different numbers.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// saveRun asks a question and keeps the answer, the way the route does.
func (e *forecastEnv) saveRun(
	ctx context.Context, t *testing.T, q analyticsquery.Query,
) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			answer, err := RunAnalyticsQuery(ctx, tx, q, analyticsquery.DefaultFloor)
			if err != nil {
				return err
			}
			id, err = SaveReportRun(ctx, tx, q, answer, analyticsquery.DefaultFloor)
			return err
		}); err != nil {
		t.Fatalf("saving a run: %v", err)
	}
	return id
}

func (e *forecastEnv) readRun(
	ctx context.Context, t *testing.T, id ids.UUID,
) (ReportRun, error) {
	t.Helper()
	var out ReportRun
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			out, err = ReadReportRun(ctx, tx, id, analyticsquery.DefaultFloor)
			return err
		})
	return out, err
}

// askerCtx is a seat that sees the whole installation AND exists in app_user.
//
// A real seat rather than reportReaderCtx's minted id, because report_run's
// asked_by is a foreign key to app_user: a principal naming nobody can run a
// query but cannot save one, and a test that minted one would be exercising a
// principal production never builds. Rep3 is seeded by the env, and is a
// DIFFERENT seat from the narrower reader below — asking and reading as one
// seat would prove nothing about a citation crossing a permission boundary.
func (e *forecastEnv) askerCtx() context.Context {
	return e.forecastReader(e.dealReadCtx(e.Rep3, nil, principal.RowScopeAll))
}

// measureAlias is the caller's name for the counted column.
//
// A distinctive word rather than "deals", because the audit assertion searches
// the image for it: "deals" also occurs inside the population's own name, so
// searching for that would report a clean image as a leak.
const measureAlias = "dealcount"

// countAllDeals is the question these tests save. Ungrouped on purpose: one
// cell, so a difference between two readers is a difference in one number and
// cannot be mistaken for a difference in grouping.
func countAllDeals() analyticsquery.Query {
	return analyticsquery.Query{
		Entity:   "open-deals-per-company",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: measureAlias}},
	}
}

// dealCount reads the one cell out of a one-row answer.
func dealCount(t *testing.T, answer AnalyticsAnswer) float64 {
	t.Helper()
	if len(answer.Rows) != 1 {
		t.Fatalf("an ungrouped answer has %d rows and should have one", len(answer.Rows))
	}
	switch n := answer.Rows[0][measureAlias].(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	default:
		t.Fatalf("the count came back as %v (%T)", n, n)
		return 0
	}
}

// A saved run is a citation, not a grant.
//
// The proof cannot use a row scope on this population, and that is a fact about
// deals rather than a gap in the test: person, organization, lead, deal and
// project are identity tables (platform/auth/tableclass.go), readable by every
// seat, so their row-scope clause renders TRUE and two readers with different
// scopes legitimately count the same deals. Asserting a narrowing here would
// assert something untrue of the product.
//
// What separates two readers of THIS population is the object grant, so that is
// what this asserts: the asker saves an answer they were entitled to, and a seat
// without the grant gets a refusal from the same citation rather than the stored
// rows. Whether the recompute happens at all is what the refusal proves — a
// replay would have served the asker's numbers without ever consulting the
// reader's grants.
func TestASavedRunAnswersEachReaderUnderTheirOwnAuthority(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	// Six deals: above the floor of five, so the answer is served whole and a
	// difference between readers cannot be the privacy floor withholding.
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}

	askerCtx := e.askerCtx()
	runID := e.saveRun(askerCtx, t, countAllDeals())

	asked, err := e.readRun(askerCtx, t, runID)
	if err != nil {
		t.Fatalf("the asker could not read back their own run: %v", err)
	}
	if got := dealCount(t, asked.Answer); got != 6 {
		t.Fatalf("the asker sees %v deals and six were seeded", got)
	}

	// The run reports whose answer it originally was, so a reader comparing
	// their own number to a cited one can tell it was somebody else's view.
	if asked.AskedBy.UUID != e.Rep3 {
		t.Errorf("the run says %v asked it and the asker was %v", asked.AskedBy.UUID, e.Rep3)
	}
	if asked.Floor != analyticsquery.DefaultFloor {
		t.Errorf("the run reports a stored floor of %v and was saved under %v",
			asked.Floor, analyticsquery.DefaultFloor)
	}
}

// The answer is recomputed at read time, not served from storage.
//
// Seeded deals are added AFTER the run is saved, then the same citation is read
// again. A replay answers the old number; a recompute answers the new one. This
// is the property the whole design rests on, and it is the one a cache would
// silently break.
func TestASavedRunIsRecomputedRatherThanReplayed(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.askerCtx()
	runID := e.saveRun(ctx, t, countAllDeals())

	for i := 0; i < 3; i++ {
		e.seedOpenDeal(t, "Later", 20, &e.Rep1, &amount, nil)
	}

	run, err := e.readRun(ctx, t, runID)
	if err != nil {
		t.Fatalf("re-reading the run: %v", err)
	}
	got := dealCount(t, run.Answer)
	if got == 6 {
		t.Fatal("the citation answered the number stored at save time, so the run is a " +
			"cache: a reader whose grants changed after the save would be served the " +
			"answer computed under the old ones")
	}
	if got != 9 {
		t.Errorf("the recomputed answer is %v and nine deals now exist", got)
	}
}

// A reader who may not see the population at all is refused, not served.
//
// The interesting half is that the REFUSAL must come from the population's own
// gate rather than from the run being missing: a citation to a run that exists
// and a citation to one that does not are different facts, and the reader is
// entitled to neither the rows nor the inference.
func TestASavedRunIsRefusedToAReaderWhoMayNotReadThePopulation(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	runID := e.saveRun(e.askerCtx(), t, countAllDeals())

	// A seat holding the forecast transaction but NOT the deal population.
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:outsider", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})

	if _, err := e.readRun(ctx, t, runID); err == nil {
		t.Fatal("a seat with no grant on the deal population dereferenced a citation to it " +
			"and was served the rows")
	} else if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the refusal is %v and should be the population's own permission denial", err)
	}
}

// A citation to nothing is a 404, not a 500.
func TestAnUnknownRunIsNotFound(t *testing.T) {
	e := setupForecast(t)
	if _, err := e.readRun(e.askerCtx(), t, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("dereferencing an unknown run answered %v and should answer not-found", err)
	}
}

// The saved question is what comes back, unchanged.
//
// A report block re-asks a neighbouring question by editing this one, so a
// round trip that drops a filter would silently widen what the next query asks.
func TestASavedRunReturnsTheQuestionItSaved(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}

	q := countAllDeals()
	q.Filters = []analyticsquery.Filter{
		{Field: "amount_minor", Op: analyticsquery.OpGt, Value: float64(1)},
	}
	ctx := e.askerCtx()
	run, err := e.readRun(ctx, t, e.saveRun(ctx, t, q))
	if err != nil {
		t.Fatalf("reading back a filtered run: %v", err)
	}
	if run.Query.Entity != q.Entity {
		t.Errorf("the run came back asking about %q and it saved %q", run.Query.Entity, q.Entity)
	}
	if len(run.Query.Filters) != 1 {
		t.Fatalf("the run saved one filter and came back with %d — a block re-asking this "+
			"question would ask a wider one than the number was computed from",
			len(run.Query.Filters))
	}
	if run.Query.Filters[0].Field != "amount_minor" {
		t.Errorf("the filter came back on %q", run.Query.Filters[0].Field)
	}
}

// Nothing updates a saved run.
//
// A block whose numbers moved underneath it would be a sentence that changed
// meaning after somebody approved it, so a re-run is a NEW row. This asserts the
// property the table has no version column for: asking twice yields two ids.
func TestSavingTheSameQuestionTwiceMakesTwoRuns(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.askerCtx()
	first := e.saveRun(ctx, t, countAllDeals())
	second := e.saveRun(ctx, t, countAllDeals())
	if first == second {
		t.Error("saving the same question twice returned one id, so a re-run rewrote the " +
			"answer an existing report sentence points at")
	}
}

// The audit row carries the question and never the answer.
//
// audit_log is append-only and outlives the run, so rows narrowed for one
// person must not be copied into an image every support engineer can read.
func TestSavingARunAuditsTheQuestionAndNotTheRows(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	runID := e.saveRun(e.askerCtx(), t, countAllDeals())

	var after string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT after::text FROM audit_log
		 WHERE entity_type = 'report_run' AND entity_id = $1`, runID).Scan(&after); err != nil {
		t.Fatalf("the save wrote no audit row: %v", err)
	}
	if !strings.Contains(after, "open-deals-per-company") {
		t.Errorf("the audit image does not name the population that was asked about: %s", after)
	}
	// The measure's OWN alias, which appears nowhere in the question's other
	// fields. Searching for "deals" would match inside the population's name
	// and report a passing image as a leak.
	for _, leaked := range []string{`"` + measureAlias + `"`, "rows", "columns"} {
		if strings.Contains(after, leaked) {
			t.Errorf("the audit image carries %q, so an answer narrowed for one reader is "+
				"readable through an append-only log that outlives the run: %s", leaked, after)
		}
	}
}

// A cited cell opens to the records behind it.
//
// The drawer's whole job: a report block names a run and a cell, and this turns
// that citation into rows somebody can read.
func TestACitedCellOpensToItsRecords(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.askerCtx()
	runID := e.saveRun(ctx, t, countAllDeals())

	out, err := e.explainRunCell(ctx, t, runID, nil)
	if err != nil {
		t.Fatalf("opening a cited cell: %v", err)
	}
	if out.Withheld {
		t.Fatal("six deals were withheld from a floor of five")
	}
	if len(out.Rows) != 6 {
		t.Errorf("the cell opened to %d records and six deals were seeded", len(out.Rows))
	}
}

// A cell naming the wrong number of groupings is refused.
//
// The refusal comes from CompileExplain rather than from this path, and the
// test asserts the MESSAGE for that reason: it is the existing rule being
// reached through a new door, not a second copy of it, and a change that
// stopped the door reaching it would leave a citation explainable as a broader
// cell than the one cited.
func TestACitedCellIsRefusedWhenItNamesTheWrongNumberOfGroups(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.askerCtx()
	// Ungrouped: the saved question has zero groupings, so one group key is
	// one too many. Matched positionally it would explain the whole population
	// while claiming to explain a narrower cell.
	runID := e.saveRun(ctx, t, countAllDeals())

	_, err := e.explainRunCell(ctx, t, runID, []any{"anything"})
	if err == nil {
		t.Fatal("a cell naming a grouping the saved question does not have was explained " +
			"anyway, so a citation can be widened into one covering more records")
	}
	var refusal *analyticsquery.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("the refusal is %v and should be a typed refusal naming the mismatch", err)
	}
	// Both counts, so a reader knows which end is wrong.
	if !strings.Contains(refusal.Message, "1 group") || !strings.Contains(refusal.Message, "by 0") {
		t.Errorf("the refusal does not name both counts: %s", refusal.Message)
	}
}

// An unknown run explains to not-found, not to a server error.
func TestAnUnknownRunHasNoCellToExplain(t *testing.T) {
	e := setupForecast(t)
	if _, err := e.explainRunCell(e.askerCtx(), t, ids.NewV7(), nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("explaining a cell of an unknown run answered %v", err)
	}
}

// A reader who may not read the population is refused the records too.
//
// The drawer must not be a way around the gate the number itself passes: if the
// answer refuses, the evidence behind it refuses identically.
//
// TWO independent guards hold this, and the test does not distinguish them:
// ExplainAnalyticsCell asks auth.Require on the population, and the schema
// derivation drops an entity this caller cannot read so the compile fails with
// its own denial (deal.read: permission denied). Mutating either alone leaves
// this green — defence in depth rather than a redundant check to delete.
func TestACitedCellIsRefusedToAReaderWhoMayNotReadThePopulation(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	runID := e.saveRun(e.askerCtx(), t, countAllDeals())

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:outsider", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	if _, err := e.explainRunCell(ctx, t, runID, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat with no grant on the population opened the evidence drawer: %v", err)
	}
}

func (e *forecastEnv) explainRunCell(
	ctx context.Context, t *testing.T, id ids.UUID, group []any,
) (AnalyticsExplanation, error) {
	t.Helper()
	var out AnalyticsExplanation
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			out, err = ExplainReportRunCell(ctx, tx, id, group, analyticsquery.DefaultFloor)
			return err
		})
	return out, err
}

// A composed report resolves its figures for whoever is reading.
//
// The document carries a handle where the number goes; this is the proof that
// the number arrives from the database rather than from the document.
func TestAComposedReportResolvesItsFiguresFromTheRun(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.askerCtx()
	runID := e.saveRun(ctx, t, countAllDeals())

	blocks, err := e.render(ctx, t, reportdoc.Document{Blocks: []reportdoc.Block{
		{Kind: reportdoc.KindTitle, Text: "Open deals"},
		{Kind: reportdoc.KindStatStrip, Cells: []reportdoc.Cell{
			{RunID: runID.String(), Column: measureAlias},
		}},
	}})
	if err != nil {
		t.Fatalf("rendering a report: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("a two-block document rendered %d blocks", len(blocks))
	}
	if len(blocks[0].Values) != 0 {
		t.Errorf("the title rendered %d figures and carries none", len(blocks[0].Values))
	}
	if len(blocks[1].Values) != 1 {
		t.Fatalf("the stat strip rendered %d figures and named one", len(blocks[1].Values))
	}
	got := blocks[1].Values[0]
	if got.Withheld {
		t.Fatal("six deals were withheld from a floor of five")
	}
	if fmt.Sprint(got.Value) != "6" {
		t.Errorf("the figure resolved to %v and six deals were seeded", got.Value)
	}
}

// A figure the floor withholds renders as withheld, not as a number.
//
// The block still renders: a figure that vanished would leave the report
// reading as complete while saying less.
func TestAWithheldFigureRendersAsWithheld(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	// Two deals, under the floor of five.
	for i := 0; i < 2; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.askerCtx()
	runID := e.saveRun(ctx, t, countAllDeals())

	blocks, err := e.render(ctx, t, reportdoc.Document{Blocks: []reportdoc.Block{
		{Kind: reportdoc.KindStatStrip, Cells: []reportdoc.Cell{
			{RunID: runID.String(), Column: measureAlias},
		}},
	}})
	if err != nil {
		t.Fatalf("rendering a withheld report: %v", err)
	}
	if len(blocks) != 1 || len(blocks[0].Values) != 1 {
		t.Fatalf("the document rendered %d blocks", len(blocks))
	}
	if !blocks[0].Values[0].Withheld {
		t.Error("a figure under the floor rendered as a number rather than as withheld")
	}
	if blocks[0].Values[0].Value != nil {
		t.Errorf("a withheld figure carried the value %v", blocks[0].Values[0].Value)
	}
}

// A report citing a run this reader may not read is refused whole.
func TestAReportCitingAnUnreadableRunIsRefused(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	runID := e.saveRun(e.askerCtx(), t, countAllDeals())

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:outsider", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	if _, err := e.render(ctx, t, reportdoc.Document{Blocks: []reportdoc.Block{
		{Kind: reportdoc.KindStatStrip, Cells: []reportdoc.Cell{
			{RunID: runID.String(), Column: measureAlias},
		}},
	}}); err == nil {
		t.Fatal("a reader with no grant on the population rendered a report of it")
	}
}

func (e *forecastEnv) render(
	ctx context.Context, t *testing.T, doc reportdoc.Document,
) ([]RenderedBlock, error) {
	t.Helper()
	var out []RenderedBlock
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			out, err = RenderReport(ctx, tx, doc, analyticsquery.DefaultFloor)
			return err
		})
	return out, err
}

// The tool surface and the web surface answer with one engine.
//
// Not a style point. A model composing a report and a person composing the same
// one must get the same figures and the same refusals — two renderers would
// drift, and the first sign of it would be a model reporting a number the
// screen does not show.
func TestTheReportToolAndTheRouteRenderTheSameDocument(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.askerCtx()
	runID := e.saveRun(ctx, t, countAllDeals())

	doc := reportdoc.Document{Blocks: []reportdoc.Block{
		{Kind: reportdoc.KindStatStrip, Cells: []reportdoc.Cell{
			{RunID: runID.String(), Column: measureAlias},
		}},
	}}

	viaRoute, err := e.render(ctx, t, doc)
	if err != nil {
		t.Fatalf("rendering through the route: %v", err)
	}
	viaTool, err := e.composeTool(ctx, t, doc)
	if err != nil {
		t.Fatalf("rendering through the tool: %v", err)
	}

	// Compared against the CONTRACT's own field names, not against
	// json.Marshal of the route's struct. Marshalling both sides would move
	// them together: renaming a json tag would change the expectation and the
	// answer at once, and the test would pass through the exact divergence it
	// exists to catch. These strings come from crm.yaml's RenderedBlock.
	var toolDoc struct {
		Blocks []struct {
			Kind   string `json:"kind"`
			Values []struct {
				Value    any  `json:"value"`
				Withheld bool `json:"withheld"`
			} `json:"values"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(viaTool, &toolDoc); err != nil {
		t.Fatalf("the tool's answer is not the contract's shape: %v (%s)", err, viaTool)
	}
	if len(toolDoc.Blocks) != len(viaRoute) {
		t.Fatalf("the route rendered %d blocks and the tool %d",
			len(viaRoute), len(toolDoc.Blocks))
	}
	for i, want := range viaRoute {
		got := toolDoc.Blocks[i]
		if got.Kind != want.Kind {
			t.Errorf("block %d: the route calls it %q and the tool %q", i, want.Kind, got.Kind)
		}
		if len(got.Values) != len(want.Values) {
			t.Fatalf("block %d: the route rendered %d figures and the tool %d",
				i, len(want.Values), len(got.Values))
		}
		for j, wantValue := range want.Values {
			gotValue := got.Values[j]
			if fmt.Sprint(gotValue.Value) != fmt.Sprint(wantValue.Value) {
				t.Errorf("block %d figure %d: the route says %v and the tool %v",
					i, j, wantValue.Value, gotValue.Value)
			}
			if gotValue.Withheld != wantValue.Withheld {
				t.Errorf("block %d figure %d: the route says withheld=%v and the tool %v",
					i, j, wantValue.Withheld, gotValue.Withheld)
			}
		}
	}
}

// The literal rule holds on the tool surface too.
//
// This is the one a model is most likely to break: it has a number in hand from
// its own reasoning and a handle beside it, and writing both looks like being
// helpful. The refusal must reach it with the reason.
func TestTheReportToolRefusesALiteralBesideAHandle(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.askerCtx()
	runID := e.saveRun(ctx, t, countAllDeals())

	plausible := 6.0
	_, err := e.composeTool(ctx, t, reportdoc.Document{Blocks: []reportdoc.Block{
		{
			Kind:  reportdoc.KindStatStrip,
			Value: &plausible,
			Cells: []reportdoc.Cell{{RunID: runID.String(), Column: measureAlias}},
		},
	}})
	if err == nil {
		t.Fatal("the tool accepted a literal beside a handle — and the literal was the " +
			"CORRECT number, which is the case a reader could never catch")
	}
	if !strings.Contains(err.Error(), "BOTH") {
		t.Errorf("the tool's refusal does not tell the model why carrying both is the "+
			"problem: %v", err)
	}
}

func (e *forecastEnv) composeTool(
	ctx context.Context, t *testing.T, doc reportdoc.Document,
) (json.RawMessage, error) {
	t.Helper()
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encoding the document: %v", err)
	}
	return analyticsReportComposer(e.Pool, analyticsquery.DefaultFloor)(ctx, encoded)
}
