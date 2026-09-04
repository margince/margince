// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A generic query against real rows.
//
// The unit tests prove the compiler renders what it should and the floor
// withholds what it should. These prove the two meet Postgres: that the SQL is
// valid, that the counts the floor judges on are the counts the database
// returned, and that a caller without the population's grant is refused.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// askAnalytics runs one query as one caller.
// askAnalytics runs one query as one caller, at the installation's own floor.
//
// The floor is not a parameter: every case here is about what the SHIPPED floor
// does, and a test that moved it would be describing a configuration no
// installation has.
func (e *forecastEnv) askAnalytics(
	ctx context.Context, t *testing.T, q analyticsquery.Query,
) (AnalyticsAnswer, error) {
	t.Helper()
	var out AnalyticsAnswer
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			out, err = RunAnalyticsQuery(ctx, tx, q, analyticsquery.DefaultFloor)
			return err
		})
	return out, err
}

// reportReaderCtx is a seat that may read the deal population and the forecast
// transaction it runs in.
func (e *forecastEnv) reportReaderCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:reader", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal": {Read: true}, "forecast": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestAGenericQueryCompilesToValidSQLAndCounts(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 7; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}

	// Seven deals in one stage: comfortably above the floor, so the answer is
	// served whole and the arithmetic is checkable.
	answer, err := e.askAnalytics(e.reportReaderCtx(), t, analyticsquery.Query{
		Entity:   "open-deals-per-company",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
	})
	if err != nil {
		t.Fatalf("a query the compiler accepted was refused by the database: %v", err)
	}
	if answer.Withheld {
		t.Error("seven deals were withheld from a floor of five")
	}
	if len(answer.Rows) != 1 {
		t.Fatalf("an ungrouped query answered %d rows and should answer one", len(answer.Rows))
	}
	if got := answer.Rows[0]["deals"]; got != float64(7) && got != int64(7) {
		t.Errorf("the count is %v (%T) and seven deals were seeded", got, got)
	}
	// The floor's own count never reaches the caller.
	if _, present := answer.Rows[0][countRowsColumnName]; present {
		t.Error("the plan's row count was served to the caller; it is the floor's input")
	}
}

// countRowsColumnName mirrors the compiler's internal name. Spelled here rather
// than exported, because the assertion is that this column is ABSENT — a test
// importing the real constant would still pass if both were renamed together,
// while what must hold is that no column by this name ships.
const countRowsColumnName = "_rows"

func TestASmallGroupIsWithheldAndSoIsItsComplement(t *testing.T) {
	e := setupForecast(t)
	big, small := int64(100_000), int64(1_000)
	// Six deals owned by one rep, two by another. Grouped by owner, the second
	// group is under the floor — and withholding it alone would leave it as
	// the total minus the first.
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Big", 20, &e.Rep1, &big, nil)
	}
	for i := 0; i < 2; i++ {
		e.seedOpenDeal(t, "Small", 20, &e.Rep3, &small, nil)
	}

	answer, err := e.askAnalytics(e.reportReaderCtx(), t, analyticsquery.Query{
		Entity: "open-deals-per-company",
		// By OWNER, which is the dimension this fixture actually splits on:
		// every deal here is in euros, so grouping by currency would be one
		// group of eight and the floor would have nothing to judge.
		GroupBy:  []string{"owner_id"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
		// Five: the six-deal group clears it and the two-deal group does not,
		// which is exactly the shape the complement rule exists for.
	})
	if err != nil {
		t.Fatalf("the grouped query was refused: %v", err)
	}
	// A floor of seven puts every group of eight-or-fewer under it, so
	// something must be withheld and the total must be unsafe.
	if !answer.Withheld {
		t.Fatal("a group under the floor was served")
	}
	if answer.TotalSafe {
		t.Error("the total is offered alongside a withheld group, which is the subtraction")
	}
	for _, row := range answer.Rows {
		if row[withheldColumn] == true && row["deals"] != nil {
			t.Errorf("a withheld row still carries its measure: %v", row)
		}
	}
}

func TestACallerWithoutThePopulationsGrantIsRefused(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)

	// forecast:read admits the transaction; deal:read does not exist on this
	// seat, so the population is refused. Without the second gate the query
	// would run — which is why the executor asks it rather than trusting that
	// an absent vocabulary is enough.
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:narrow", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"forecast": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})

	_, err := e.askAnalytics(ctx, t, analyticsquery.Query{
		Entity:   "open-deals-per-company",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
	})
	if err == nil {
		t.Fatal("a seat without deal:read read the deal population")
	}
	// Either refusal is correct and they say different things: the population
	// may be absent from this seat's vocabulary (unsupported), or present and
	// gated (permission denied). What must not happen is an answer.
	var refusal *analyticsquery.RefusalError
	if !errors.As(err, &refusal) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the refusal was %v, which is neither a typed refusal nor a denial", err)
	}
}

func TestAFilterThatSeparatesOutTooFewRecordsIsRefused(t *testing.T) {
	e := setupForecast(t)
	big, small := int64(100_000), int64(1_000)
	// Six deals for one rep, two for another. Each ungrouped answer covers a
	// group of eight or six, comfortably above the floor — so neither is
	// withheld, and their difference is the second rep's exact figure.
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Big", 20, &e.Rep1, &big, nil)
	}
	for i := 0; i < 2; i++ {
		e.seedOpenDeal(t, "Small", 20, &e.Rep3, &small, nil)
	}
	ctx := e.reportReaderCtx()

	// Leg one: the whole population. Served, and correctly — eight deals.
	whole, err := e.askAnalytics(ctx, t, analyticsquery.Query{
		Entity: "open-deals-per-company",
		Measures: []analyticsquery.Measure{
			{Fn: analyticsquery.CountAll, As: "n"},
			{Fn: analyticsquery.Sum, Field: "amount_minor", As: "total"},
		},
	})
	if err != nil {
		t.Fatalf("the unfiltered question was refused: %v", err)
	}
	if whole.Withheld {
		t.Fatal("eight deals in one group were withheld from a floor of five")
	}

	// Leg two: the same question, with the two-deal rep filtered out. THIS is
	// the one that must not be answered — subtracting it from leg one gives
	// that rep's exact count and exact revenue.
	_, err = e.askAnalytics(ctx, t, analyticsquery.Query{
		Entity: "open-deals-per-company",
		Filters: []analyticsquery.Filter{
			{Field: "owner_id", Op: analyticsquery.OpNe, Value: e.Rep3.String()},
		},
		Measures: []analyticsquery.Measure{
			{Fn: analyticsquery.CountAll, As: "n"},
			{Fn: analyticsquery.Sum, Field: "amount_minor", As: "total"},
		},
	})
	if err == nil {
		t.Fatal("a filter excluding two deals was answered; set beside the unfiltered answer it describes them exactly")
	}
	var refusal *analyticsquery.RefusalError
	if !errors.As(err, &refusal) || refusal.Kind != analyticsquery.RefusalPrivacy {
		t.Fatalf("the refusal was %v; it is a privacy refusal", err)
	}
	// And the refusal does not name the number, which would be the disclosure.
	if strings.Contains(refusal.Error(), " 2 ") {
		t.Errorf("the refusal states how many records the filter excluded: %s", refusal.Error())
	}
}

func TestAFilterThatSeparatesOutEnoughIsStillAnswered(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	// Six and six. Filtering either out excludes six, which clears the floor,
	// so the question is ordinary and must still be answered — a guard that
	// refused this would refuse most real filtering.
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &amount, nil)
		e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, &amount, nil)
	}

	answer, err := e.askAnalytics(e.reportReaderCtx(), t, analyticsquery.Query{
		Entity: "open-deals-per-company",
		Filters: []analyticsquery.Filter{
			{Field: "owner_id", Op: analyticsquery.OpNe, Value: e.Rep3.String()},
		},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
	})
	if err != nil {
		t.Fatalf("a filter excluding six deals was refused: %v", err)
	}
	if answer.Withheld {
		t.Error("a six-deal answer was withheld from a floor of five")
	}
}

func TestAWithheldGroupKeepsNoKeysEither(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	// One deal per owner: every group is of size one, so every group is under
	// the floor. Keeping the keys would make this a paginated dump of who owns
	// deals, with the counts blanked and the identities handed over.
	for i := 0; i < 4; i++ {
		owner := ids.NewV7()
		if _, err := e.owner.Exec(context.Background(),
			`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Owner')`,
			owner, owner.String()+"@forecast.test"); err != nil {
			t.Fatal(err)
		}
		e.seedOpenDeal(t, "Deal", 20, &owner, &amount, nil)
	}

	answer, err := e.askAnalytics(e.reportReaderCtx(), t, analyticsquery.Query{
		Entity:   "open-deals-per-company",
		GroupBy:  []string{"owner_id"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
	})
	if err != nil {
		t.Fatalf("the grouped question was refused: %v", err)
	}
	if !answer.Withheld {
		t.Fatal("four groups of one were served whole")
	}
	for _, row := range answer.Rows {
		if row[withheldColumn] != true {
			continue
		}
		if row["owner_id"] != nil {
			t.Errorf("a withheld group still names its owner: %v", row["owner_id"])
		}
		if row["n"] != nil {
			t.Errorf("a withheld group still carries its count: %v", row["n"])
		}
	}
}

// explainCell opens one cell of an answer.
func (e *forecastEnv) explainCell(
	ctx context.Context, t *testing.T, in analyticsquery.Explain,
) (AnalyticsExplanation, error) {
	t.Helper()
	var out AnalyticsExplanation
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			out, err = ExplainAnalyticsCell(ctx, tx, in, analyticsquery.DefaultFloor)
			return err
		})
	return out, err
}

func TestACellOpensToTheRecordsItWasComputedFrom(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.reportReaderCtx()
	q := analyticsquery.Query{
		Entity:   "open-deals-per-company",
		GroupBy:  []string{"owner_id"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
	}

	answer, err := e.askAnalytics(ctx, t, q)
	if err != nil {
		t.Fatalf("the question was refused: %v", err)
	}
	if answer.Withheld {
		t.Fatal("six deals in one group were withheld from a floor of five")
	}

	explanation, err := e.explainCell(ctx, t, analyticsquery.Explain{
		Query: q, Group: []any{e.Rep1.String()},
	})
	if err != nil {
		t.Fatalf("opening the cell was refused: %v", err)
	}
	if explanation.Withheld {
		t.Fatal("a served cell explained to nothing")
	}
	// The explanation reads the SAME rows the number counted. A drill-through
	// that showed more would be showing rows excluded from the total above it.
	if len(explanation.Rows) != 6 {
		t.Errorf("the cell counted six deals and opened to %d records", len(explanation.Rows))
	}
	if explanation.Truncated {
		t.Error("six records were reported as a truncated page")
	}
}

func TestAWithheldCellExplainsToWithheldRatherThanToNothing(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	// Two deals for one rep. Under the floor, so the answer withholds the
	// group — and opening it record by record would be that same disclosure at
	// a slower pace.
	for i := 0; i < 2; i++ {
		e.seedOpenDeal(t, "Small", 20, &e.Rep3, &amount, nil)
	}
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Big", 20, &e.Rep1, &amount, nil)
	}
	ctx := e.reportReaderCtx()
	q := analyticsquery.Query{
		Entity:   "open-deals-per-company",
		GroupBy:  []string{"owner_id"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
	}
	if _, err := e.askAnalytics(ctx, t, q); err != nil {
		t.Fatalf("the question was refused: %v", err)
	}

	explanation, err := e.explainCell(ctx, t, analyticsquery.Explain{
		Query: q, Group: []any{e.Rep3.String()},
	})
	if err != nil {
		t.Fatalf("opening a withheld cell errored rather than answering withheld: %v", err)
	}
	if !explanation.Withheld {
		t.Fatal("a withheld cell opened to its records")
	}
	if len(explanation.Rows) != 0 {
		t.Errorf("a withheld cell returned %d records", len(explanation.Rows))
	}
}

func TestAnExplanationCannotOutSeeTheNumberItExplains(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 6; i++ {
		e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &amount, nil)
		e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, &amount, nil)
	}
	q := analyticsquery.Query{
		Entity:   "open-deals-per-company",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
	}

	// A fully masked reader counts nothing, because every row leaves the money
	// through the mask exclusion. Their explanation must be empty too — an
	// explanation showing rows the aggregate excluded is the drill-through
	// disclosing what the total was narrowed to hide.
	masked := e.forecastReader(e.maskedDealReader(principal.MaskAlways))
	answer, err := e.askAnalytics(masked, t, q)
	if err != nil {
		t.Fatalf("the masked reader's question was refused: %v", err)
	}
	explanation, err := e.explainCell(masked, t, analyticsquery.Explain{Query: q})
	if err != nil {
		t.Fatalf("the masked reader's explanation was refused: %v", err)
	}
	if len(explanation.Rows) != 0 {
		t.Errorf("a masked reader whose answer counted %v opened it to %d records",
			answer.Rows, len(explanation.Rows))
	}
}

// ownLensRepCtx is a rep who may READ deals and may measure only their own.
//
// The two are separate, and that separation is the whole defect: deals are
// workspace-readable by design, so record authorization admits every row and a
// population that consulted only row scope answered a rep with the whole
// installation's numbers.
func (e *forecastEnv) ownLensRepCtx(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: user,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal": {Read: true}, "forecast": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

// A rep asking a typed question is asking about THEIR work.
//
// Seven of their own and seven of a colleague on another team's, both above the floor, so
// neither count is withheld and the number that comes back says which
// population was measured rather than which rows survived a floor.
func TestATypedQueryAnswersTheAskersOwnPopulation(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 7; i++ {
		e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &amount, nil)
		e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, &amount, nil)
	}

	answer, err := e.askAnalytics(e.ownLensRepCtx(e.Rep1), t, analyticsquery.Query{
		Entity:   "open-deals-per-company",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
	})
	if err != nil {
		t.Fatalf("a rep's own question was refused: %v", err)
	}
	if len(answer.Rows) != 1 {
		t.Fatalf("an ungrouped query answered %d rows: %+v", len(answer.Rows), answer.Rows)
	}
	if got := answer.Rows[0]["deals"]; got != float64(7) && got != int64(7) {
		t.Errorf("the rep's count is %v, want their own 7 of the 14 seeded — a count of 14 "+
			"is the whole installation answered to somebody who may measure their own work", got)
	}
}

// Asking for the workspace is REFUSED, not quietly narrowed.
//
// Narrowing would answer a different question than the one asked, with nothing
// on the answer saying so — and a caller who could assert a wider population
// and be silently given a smaller one has no way to tell the two apart from a
// genuinely small one.
func TestATypedQueryRefusesAPopulationTheAskerCannotMeasure(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 7; i++ {
		e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, &amount, nil)
	}

	_, err := e.askAnalytics(e.ownLensRepCtx(e.Rep1), t, analyticsquery.Query{
		Entity:    "open-deals-per-company",
		ScopeKind: ScopeKindWorkspace,
		Measures:  []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("asking for the whole workspace answered %v, want a refusal — a caller "+
			"given a narrower answer than they asked for cannot tell it from a small one", err)
	}
}

// wideLensCtx is a REAL seat that may measure the whole workspace.
//
// A real one because saving a run records who asked, and report_run carries a
// foreign key to app_user — the synthetic id reportReaderCtx mints is fine for
// asking a question and cannot be stored beside one.
func (e *forecastEnv) wideLensCtx(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:lead", UserID: user,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal": {Read: true}, "forecast": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A saved run answers under whoever OPENS it.
//
// The stored question carries the scope that was asked for, never the one that
// was resolved. Storing the resolution would make a saved link a way to read
// somebody else's numbers: a manager saves a run and the link hands their
// answer to every rep who opens it. It is the same rule the floor already
// follows here — the stored one is reported for comparison and never used to
// judge the answer being served.
func TestASavedRunIsReAskedUnderTheReadersOwnLens(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := 0; i < 7; i++ {
		e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &amount, nil)
		e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, &amount, nil)
	}
	q := analyticsquery.Query{
		Entity:    "open-deals-per-company",
		ScopeKind: ScopeKindWorkspace,
		Measures:  []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
	}

	// Saved by a seat that MAY measure the workspace, and the answer says 14.
	wide := e.wideLensCtx(e.Rep3)
	answer, err := e.askAnalytics(wide, t, q)
	if err != nil {
		t.Fatalf("the wider seat's question was refused: %v", err)
	}
	if got := answer.Rows[0]["deals"]; got != float64(14) && got != int64(14) {
		t.Fatalf("the wide answer is %v, want 14 — the rest of this case compares "+
			"against it and says nothing if it was not the whole installation", got)
	}
	var runID ids.UUID
	if err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(wide,
		func(ctx context.Context, tx pgx.Tx) error {
			var saveErr error
			runID, saveErr = SaveReportRun(ctx, tx, q, answer, analyticsquery.DefaultFloor)
			return saveErr
		}); err != nil {
		t.Fatalf("saving the run: %v", err)
	}

	// Opened by a rep who may measure only their own. The stored question asks
	// for the workspace, and that is now a scope THIS reader may not have — so
	// the refusal is the same one they would meet asking directly.
	// The read's own refusal is what this asserts, so it is captured rather
	// than returned: returning it would roll the transaction back and report
	// the same error through a second path, and a transaction failure would
	// then be indistinguishable from the refusal being looked for.
	var readErr error
	if err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(e.ownLensRepCtx(e.Rep1),
		func(ctx context.Context, tx pgx.Tx) error {
			_, readErr = ReadReportRun(ctx, tx, runID, analyticsquery.DefaultFloor)
			return nil
		}); err != nil {
		t.Fatalf("opening the saved run: %v", err)
	}
	if !errors.Is(readErr, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep opening a workspace-scoped run answered %v, want the refusal "+
			"they would get asking it themselves — a saved link that answers wider "+
			"than its reader is a way to read somebody else's numbers", readErr)
	}
}
