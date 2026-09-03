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
