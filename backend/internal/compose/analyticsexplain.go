// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Opening one cell of a generic analytics answer.
//
// The rule that decides everything here: the explanation reads the SAME rows
// the number was computed from, under the same authority. It runs the same
// narrowings through specNarrowings, and it re-judges the cell against the
// floor before returning anything — because a withheld cell explained record by
// record is the same disclosure at a slower pace.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// AnalyticsExplanation is one cell's records.
type AnalyticsExplanation struct {
	Columns []string
	Rows    []map[string]any
	// Withheld says the cell itself was withheld, so there is nothing to open.
	// The rows are empty in that case and the two are different facts: an
	// empty explanation of a served cell means the group had no records, which
	// cannot happen, and reading one as the other would hide the refusal.
	Withheld bool
	// Truncated says the cell covers more records than were returned. A reader
	// who sums the rows and finds less than the cell needs to know why.
	Truncated bool
	// PartlyWithheld says an every-cell explanation left out the records of
	// cells the answer withheld or did not serve.
	PartlyWithheld bool
	// PopulationNarrowed is the answer's, said again for its records.
	PopulationNarrowed string
	// Entity is the record type each row's id names, and Question the question
	// explained, whose group columns may name other records: both for labelling.
	Entity   datasource.EntityType
	Question analyticsquery.Query
}

// ExplainAnalyticsCell resolves one cell to its records.
func ExplainAnalyticsCell(
	ctx context.Context, tx pgx.Tx, in analyticsquery.Explain, floor analyticsquery.Floor,
) (AnalyticsExplanation, error) {
	spec, ok := prebuiltReports[in.Query.Entity]
	if !ok {
		return AnalyticsExplanation{}, &analyticsquery.RefusalError{
			Kind:    analyticsquery.RefusalUnsupported,
			Message: "no population by that name",
			Suggest: "explain a cell of an answer this seat can ask for",
		}
	}
	if err := auth.Require(ctx, string(spec.entity), principal.ActionRead); err != nil {
		return AnalyticsExplanation{}, err
	}

	// The cell's own size, judged before anything is opened. Asked by RUNNING
	// the question again rather than trusting a size the caller sent: a caller
	// who could assert their cell was large would have turned the floor off.
	served, err := servedCellsFor(ctx, tx, in, floor)
	if err != nil {
		return AnalyticsExplanation{}, err
	}
	if served.none {
		return AnalyticsExplanation{Withheld: true}, nil
	}

	schema := AnalyticsSchemaFor(ctx)
	// The SAME population the count was taken over — the Explain carries the
	// whole query, scope included. A drill-through narrowed differently from
	// its own headline opens records the number never counted.
	var narrowed string
	plan, err := analyticsquery.CompileExplain(
		in, schema, analyticsScope(ctx, tx, spec, in.Query, &narrowed))
	if err != nil {
		return AnalyticsExplanation{}, err
	}
	frame, err := readReportFrame(ctx, tx)
	if err != nil {
		return AnalyticsExplanation{}, err
	}
	sql, args, err := bindReportTokens(ctx, frame, plan.SQL, plan.Args)
	if err != nil {
		return AnalyticsExplanation{}, err
	}

	pgRows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return AnalyticsExplanation{}, fmt.Errorf("compose: explaining an analytics cell: %w", err)
	}
	defer pgRows.Close()
	rows, err := scanReportRows(pgRows, plan.Columns)
	if err != nil {
		return AnalyticsExplanation{}, fmt.Errorf("compose: reading an explanation: %w", err)
	}
	opened, dropped := served.open(rows)
	return AnalyticsExplanation{
		Columns: plan.Columns, Rows: opened,
		Truncated:          len(rows) == analyticsquery.ExplainRowLimit,
		PartlyWithheld:     served.partly || dropped,
		PopulationNarrowed: narrowed,
		Entity:             spec.entity,
		Question:           in.Query,
	}, nil
}

// servedCells is what the floor lets an explanation open.
type servedCells struct {
	// none: nothing may be opened.
	none bool
	// partly: some cells of an every-cell explanation were withheld.
	partly bool
	// cells are the answer rows an every-cell explanation may open; nil for a
	// one-cell explanation, which the floor judged whole.
	cells []analyticsquery.Explain
}

// servedCellsFor asks the floor about the cell explained, or with AllCells
// about every cell of the answer, which is re-run for the reason
// cellIsWithheld gives.
func servedCellsFor(
	ctx context.Context, tx pgx.Tx, in analyticsquery.Explain, floor analyticsquery.Floor,
) (servedCells, error) {
	if !in.AllCells {
		withheld, err := cellIsWithheld(ctx, tx, in, floor)
		return servedCells{none: withheld}, err
	}
	answer, err := RunAnalyticsQuery(ctx, tx, in.Query, floor)
	if err != nil {
		return servedCells{}, err
	}
	out := servedCells{partly: answer.Withheld, cells: []analyticsquery.Explain{}}
	for _, row := range answer.Rows {
		if row[withheldColumn] == true {
			continue
		}
		cell := analyticsquery.Explain{Query: in.Query}
		for _, name := range in.Query.GroupBy {
			cell.Group = append(cell.Group, row[name])
		}
		out.cells = append(out.cells, cell)
	}
	out.none = len(out.cells) == 0
	return out, nil
}

// open keeps the explained rows that fall in a served cell, and says whether
// any fell outside one. A one-cell explanation keeps every row.
func (s servedCells) open(rows []map[string]any) ([]map[string]any, bool) {
	if s.cells == nil {
		return rows, false
	}
	kept := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if slices.ContainsFunc(s.cells, func(cell analyticsquery.Explain) bool {
			return cellMatchesRow(cell, row)
		}) {
			kept = append(kept, row)
		}
	}
	return kept, len(kept) < len(rows)
}

// cellIsWithheld re-runs the question and asks the floor about THIS cell.
//
// The whole answer rather than one group, because the floor's decision about a
// group depends on the others: a group is withheld for being small, and its
// complements go with it so it cannot be recovered by subtraction. Asking about
// one group in isolation would serve a complement the answer withheld.
func cellIsWithheld(
	ctx context.Context, tx pgx.Tx, in analyticsquery.Explain, floor analyticsquery.Floor,
) (bool, error) {
	answer, err := RunAnalyticsQuery(ctx, tx, in.Query, floor)
	if err != nil {
		return false, err
	}
	if !answer.Withheld {
		return false, nil
	}
	// Nothing left to match against: every group of the answer was withheld,
	// including whichever one this is.
	if len(in.Query.GroupBy) == 0 {
		return true, nil
	}
	for _, row := range answer.Rows {
		if !cellMatchesRow(in, row) {
			continue
		}
		return row[withheldColumn] == true, nil
	}
	// The cell is in no row of the answer. A group that does not appear is one
	// the caller did not get, so there is nothing to explain and saying so
	// beats an empty list that reads as "no records".
	return true, nil
}

// cellMatchesRow answers whether a row is the cell being explained.
//
// A withheld row carries nil for every key, so it matches nothing here — which
// is correct and is why the fallthrough above answers withheld: a cell the
// caller can name but cannot find in the answer is one that was withheld from
// them, key and all.
func cellMatchesRow(in analyticsquery.Explain, row map[string]any) bool {
	for i, name := range in.Query.GroupBy {
		if fmt.Sprint(row[name]) != fmt.Sprint(in.Group[i]) {
			return false
		}
	}
	return true
}
