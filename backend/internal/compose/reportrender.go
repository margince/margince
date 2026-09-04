// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Turning a composed report's handles into figures.
//
// The document says WHERE each number lives. This resolves each cited run the
// way reading a run directly resolves it — the saved question re-asked under
// this caller's own authority, re-judged against the current floor — and then
// reads the named cell out of the answer.
//
// So one document shows different figures to readers who may see different
// populations, and a cell the floor withheld renders as withheld rather than
// as a number. That is the same property a saved run has, reached through a
// second door, and it is why a report can be handed to somebody without
// handing them the data behind it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RenderedValue is one figure as this reader may see it.
// The json tags are the WIRE SHAPE, and both surfaces serve it. Without them
// the tool surface answered Go field names (`Value`, `Withheld`) while the HTTP
// handler mapped the same struct to its contract's snake_case — two surfaces
// disagreeing about one document, which is the drift the seam exists to
// prevent.
type RenderedValue struct {
	// Value is nil for a withheld figure AND for a cell that resolved to
	// nothing. Withheld tells the two apart, which matters: one is a number
	// somebody may not see, the other is a number that does not exist.
	Value    any  `json:"value,omitempty"`
	Withheld bool `json:"withheld"`
}

// RenderedBlock is one block with its figures filled in.
type RenderedBlock struct {
	Kind     string          `json:"kind"`
	Text     string          `json:"text,omitempty"`
	Severity string          `json:"severity,omitempty"`
	Values   []RenderedValue `json:"values"`
}

// RenderReport resolves every figure a document cites.
//
// Each run is read ONCE however many cells cite it. Not for speed: reading a
// run re-runs its question, and two reads of one run inside a single render
// could disagree if a row changed between them — the same document would then
// show two different numbers for one cell.
func RenderReport(
	ctx context.Context, tx pgx.Tx, doc reportdoc.Document, floor analyticsquery.Floor,
) ([]RenderedBlock, error) {
	runIDs, err := reportdoc.Validate(doc)
	if err != nil {
		return nil, err
	}

	answers := make(map[ids.UUID]AnalyticsAnswer, len(runIDs))
	for _, id := range runIDs {
		// The reader's own authority, not the composer's. A run the composer
		// could read and this reader cannot refuses here, and the refusal is
		// the whole document's — a report that rendered the blocks a reader
		// may see and dropped the rest would read as complete while saying
		// less.
		run, err := ReadReportRun(ctx, tx, id, floor)
		if err != nil {
			return nil, err
		}
		answers[id] = run.Answer
	}

	out := make([]RenderedBlock, 0, len(doc.Blocks))
	for _, b := range doc.Blocks {
		rendered := RenderedBlock{
			Kind: string(b.Kind), Text: b.Text, Severity: string(b.Severity),
			// Empty, never nil: a block with no figures marshals to [] rather
			// than null, and a reader parsing null as "unknown" would be
			// wrong about a block that simply shows prose.
			Values: []RenderedValue{},
		}
		for _, c := range b.Cells {
			id, err := ids.Parse(c.RunID)
			if err != nil {
				// Unreachable: Validate parsed every run id already. Returned
				// rather than ignored because a swallowed error here would
				// render a figure of nothing as though it were data.
				return nil, fmt.Errorf("compose: a validated report cell names an unreadable run: %w", err)
			}
			rendered.Values = append(rendered.Values, valueFromAnswer(answers[id], c))
		}
		out = append(out, rendered)
	}
	return out, nil
}

// valueFromAnswer reads one cell out of a resolved answer.
//
// A cell nothing matches answers withheld=false and value=nil, which is the
// honest report of "no such group in this reader's data" — different from a
// group the floor kept back, and different again from a figure of zero.
func valueFromAnswer(answer AnalyticsAnswer, c reportdoc.Cell) RenderedValue {
	for _, row := range answer.Rows {
		if !rowMatchesGroup(row, answer.Columns, c.Group) {
			continue
		}
		// A withheld row lost its values AND its keys, so this reads the row's
		// own marker rather than inferring withholding from a nil value: a
		// null measure on a served row is a real absence and must not be
		// reported as something kept back.
		if withheld, _ := row[withheldColumn].(bool); withheld {
			return RenderedValue{Withheld: true}
		}
		return RenderedValue{Value: row[c.Column]}
	}
	return RenderedValue{}
}

// rowMatchesGroup says whether a row is the cell the block named.
//
// The group keys are compared as their WIRE values, which is what both sides
// already are: the answer's rows came back through the same JSON shaping the
// document's group keys were written in, so a number is a float on both sides
// and a comparison here needs no type table that would drift from it.
func rowMatchesGroup(row map[string]any, columns []string, group []any) bool {
	if len(group) == 0 {
		return true
	}
	// The group binds the leading columns, in the answer's own order, because
	// that is the order the query grouped by and the order the cell's keys
	// were listed in.
	for i, want := range group {
		if i >= len(columns) {
			return false
		}
		if fmt.Sprint(row[columns[i]]) != fmt.Sprint(want) {
			return false
		}
	}
	return true
}
