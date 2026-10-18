// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A grouping the vocabulary offers must be able to answer. The floor withholds
// every group below it, so a field that differs for every row can only ever say
// "withheld". The corpus is the vocabulary itself: every offered grouping of
// every population that has enough rows to be judged.

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedDistinctProjects plants n delivering projects that share a company, an
// owner and a phase and differ in everything that is the row's own: id, name,
// key and both timestamps.
func seedDistinctProjects(t *testing.T, e *integration.Env, n int) {
	t.Helper()
	company := ids.NewV7()
	e.WsExec(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Grouping Co', 'manual', 'human:x')`, company)
	for i := range n {
		e.WsExec(t, `INSERT INTO project (id, name, key, company_id, owner_id, phase, source, captured_by, created_at, last_activity_at)
			VALUES ($1, $2, $3, $4, $5, 'delivering', 'manual', 'human:x',
				now() - make_interval(days => $6), now() - make_interval(days => $6, hours => 1))`,
			ids.NewV7(), fmt.Sprintf("Distinct project %d", i), fmt.Sprintf("DP-%d", i), company, e.Rep1, 30+i)
	}
}

func TestEveryGroupingTheVocabularyOffersCanAnswer(t *testing.T) {
	e := integration.Setup(t)
	floor := int(analyticsquery.DefaultFloor)
	seedDistinctProjects(t, e, 3*floor)
	ctx := e.Admin()
	store := forecasting.NewStore(InstallationDB(e.Pool))
	ask := func(q analyticsquery.Query) AnalyticsAnswer {
		t.Helper()
		var out AnalyticsAnswer
		err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			out, err = RunAnalyticsQuery(ctx, tx, q, analyticsquery.DefaultFloor)
			return err
		})
		if err != nil {
			t.Fatalf("%+v: %v", q, err)
		}
		return out
	}
	count := []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}}

	judged := map[string]bool{}
	for name, entity := range AnalyticsSchemaFor(ctx).Entities {
		total := ask(analyticsquery.Query{Entity: name, Measures: count})
		if len(total.Rows) != 1 || total.Rows[0]["n"] == nil {
			continue
		}
		population, err := strconv.Atoi(fmt.Sprint(total.Rows[0]["n"]))
		if err != nil || population < 2*floor {
			continue
		}
		judged[name] = true
		for _, field := range entity.FieldNames(analyticsquery.KindDimension) {
			answer := ask(analyticsquery.Query{Entity: name, GroupBy: []string{field}, Measures: count})
			answered := slices.ContainsFunc(answer.Rows, func(row map[string]any) bool { return row["n"] != nil })
			if !answered {
				t.Errorf("%s offers grouping by %s, and over %d rows every group is withheld", name, field, population)
			}
		}
	}
	// A field offered only to narrow by must narrow without refusing: a bound
	// that every row meets leaves the whole population to answer.
	quiet := AnalyticsSchemaFor(ctx).Entities["projects-gone-quiet"]
	for _, field := range quiet.FieldNames(analyticsquery.KindFilter) {
		answer := ask(analyticsquery.Query{
			Entity: "projects-gone-quiet", Measures: count,
			Filters: []analyticsquery.Filter{{Field: field, Op: analyticsquery.OpIsNotNull}},
		})
		if len(answer.Rows) != 1 || answer.Rows[0]["n"] == nil {
			t.Errorf("projects-gone-quiet offers narrowing by %s, and the answer is withheld: %+v", field, answer.Rows)
		}
	}
	for _, name := range []string{"project-commitments", "projects-gone-quiet"} {
		if !judged[name] {
			t.Errorf("%s was not judged: the sweep reads a smaller vocabulary than the one offered", name)
		}
	}
}
