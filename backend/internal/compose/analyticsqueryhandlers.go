// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two routes a generic analytics question needs: what may be asked, and
// the asking.
//
// Both run in one transaction so the vocabulary a query is validated against is
// the one it runs under. Derived across two transactions, a grant removed in
// between would let a plan compile against a field the run no longer admits.

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// analyticsQueryHandlers serves the schema and the query.
type analyticsQueryHandlers struct {
	db *database.DB
	// floor is the installation's group floor, injected so a test can move it
	// without editing a setting.
	floor analyticsquery.Floor
}

func newAnalyticsQueryHandlers(db *database.DB, floor analyticsquery.Floor) analyticsQueryHandlers {
	return analyticsQueryHandlers{db: db, floor: floor}
}

// GetAnalyticsSchema implements GET /analytics/schema.
func (h analyticsQueryHandlers) GetAnalyticsSchema(w http.ResponseWriter, r *http.Request) {
	schema := AnalyticsSchemaFor(r.Context())
	out := crmcontracts.AnalyticsSchema{Version: schema.Version}
	for _, name := range schema.EntityNames() {
		entity := schema.Entities[name]
		out.Entities = append(out.Entities, crmcontracts.AnalyticsEntity{
			Name:     name,
			GroupBy:  entity.FieldNames(analyticsquery.KindDimension),
			Measures: entity.FieldNames(analyticsquery.KindMeasure),
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// RunAnalyticsQuery implements POST /analytics/query.
func (h analyticsQueryHandlers) RunAnalyticsQuery(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.AnalyticsQuery
	if !httperr.Decode(w, r, &body) {
		return
	}
	q := queryFromWire(body)

	ctx := r.Context()
	var (
		answer AnalyticsAnswer
		runID  *ids.UUID
	)
	if err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		answer, err = RunAnalyticsQuery(ctx, tx, q, h.floor)
		if err != nil {
			return err
		}
		if body.Save == nil || !*body.Save {
			return nil
		}
		// Saved in the SAME transaction that produced it. Saved afterwards, a
		// failure between the two would answer with a run_id naming a row that
		// does not exist — a citation that resolves to nothing.
		id, err := SaveReportRun(ctx, tx, q, answer, h.floor)
		if err != nil {
			return err
		}
		runID = &id
		return nil
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.AnalyticsAnswer{
		Columns: answer.Columns, Rows: answer.Rows,
		Withheld: answer.Withheld, TotalSafe: answer.TotalSafe,
		SchemaVersion: answer.SchemaVersion,
	}
	if runID != nil {
		saved := openapi_types.UUID(*runID)
		out.RunId = &saved
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetReportRun implements GET /analytics/runs/{run_id}.
func (h analyticsQueryHandlers) GetReportRun(
	w http.ResponseWriter, r *http.Request, runID openapi_types.UUID,
) {
	ctx := r.Context()
	var run ReportRun
	if err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		// The caller's floor, not the stored one. A run saved under a laxer
		// floor must not serve rows this installation would withhold today.
		run, err = ReadReportRun(ctx, tx, ids.UUID(runID), h.floor)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ReportRun{
		Id:    openapi_types.UUID(run.ID),
		Query: wireFromQuery(run.Query),
		Answer: crmcontracts.AnalyticsAnswer{
			Columns: run.Answer.Columns, Rows: run.Answer.Rows,
			Withheld: run.Answer.Withheld, TotalSafe: run.Answer.TotalSafe,
			SchemaVersion: run.Answer.SchemaVersion,
		},
		AskedBy:     openapi_types.UUID(run.AskedBy.UUID),
		StoredFloor: int(run.Floor),
	})
}

// queryFromWire converts the request without validating it.
//
// Validation belongs to the compiler, which is the thing that renders SQL: a
// second check here would be a second answer to what is askable, and the two
// would drift the first time a measure was added.
func queryFromWire(in crmcontracts.AnalyticsQuery) analyticsquery.Query {
	out := analyticsquery.Query{Entity: in.Entity}
	if in.ScopeKind != nil {
		out.ScopeKind = *in.ScopeKind
	}
	if in.ScopeId != nil {
		out.ScopeID = in.ScopeId.String()
	}
	if in.GroupBy != nil {
		out.GroupBy = *in.GroupBy
	}
	if in.Limit != nil {
		out.Limit = *in.Limit
	}
	for _, m := range in.Measures {
		measure := analyticsquery.Measure{Fn: analyticsquery.AggFn(m.Fn)}
		if m.Field != nil {
			measure.Field = *m.Field
		}
		if m.As != nil {
			measure.As = *m.As
		}
		out.Measures = append(out.Measures, measure)
	}
	if in.Filters != nil {
		for _, f := range *in.Filters {
			out.Filters = append(out.Filters, analyticsquery.Filter{
				Field: f.Field, Op: analyticsquery.FilterOp(f.Op), Value: f.Value,
			})
		}
	}
	return out
}

// ExplainReportRunCell implements POST /analytics/runs/{run_id}/cells/explain.
func (h analyticsQueryHandlers) ExplainReportRunCell(
	w http.ResponseWriter, r *http.Request, runID openapi_types.UUID,
) {
	var body crmcontracts.ReportRunCell
	if !httperr.Decode(w, r, &body) {
		return
	}
	var group []any
	if body.Group != nil {
		group = *body.Group
	}

	ctx := r.Context()
	var out AnalyticsExplanation
	if err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = ExplainReportRunCell(ctx, tx, ids.UUID(runID), group, h.floor)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AnalyticsExplanation{
		Columns: out.Columns, Rows: out.Rows,
		Withheld: out.Withheld, Truncated: out.Truncated,
	})
}

// RenderAnalyticsReport implements POST /analytics/reports/render.
func (h analyticsQueryHandlers) RenderAnalyticsReport(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.ReportDocument
	if !httperr.Decode(w, r, &body) {
		return
	}

	ctx := r.Context()
	var blocks []RenderedBlock
	if err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		blocks, err = RenderReport(ctx, tx, documentFromWire(body), h.floor)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}

	out := crmcontracts.RenderedReport{Blocks: make([]crmcontracts.RenderedBlock, 0, len(blocks))}
	for _, b := range blocks {
		block := crmcontracts.RenderedBlock{
			Kind:   b.Kind,
			Values: make([]crmcontracts.RenderedValue, 0, len(b.Values)),
		}
		if b.Text != "" {
			text := b.Text
			block.Text = &text
		}
		if b.Severity != "" {
			severity := b.Severity
			block.Severity = &severity
		}
		for _, v := range b.Values {
			value := crmcontracts.RenderedValue{Withheld: v.Withheld}
			if v.Value != nil {
				held := v.Value
				value.Value = &held
			}
			block.Values = append(block.Values, value)
		}
		out.Blocks = append(out.Blocks, block)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// documentFromWire converts the request without validating it.
//
// Validation belongs to reportdoc, which is the thing that decides what may
// render. A second check here would be a second answer to what a report may
// contain, and the two would drift the first time a block was added.
func documentFromWire(in crmcontracts.ReportDocument) reportdoc.Document {
	out := reportdoc.Document{Blocks: make([]reportdoc.Block, 0, len(in.Blocks))}
	for _, b := range in.Blocks {
		block := reportdoc.Block{Kind: reportdoc.Kind(b.Kind), Value: b.Value}
		if b.Text != nil {
			block.Text = *b.Text
		}
		if b.Severity != nil {
			block.Severity = reportdoc.Severity(*b.Severity)
		}
		if b.Cells != nil {
			for _, c := range *b.Cells {
				cell := reportdoc.Cell{RunID: c.RunId.String(), Column: c.Column}
				if c.Group != nil {
					cell.Group = *c.Group
				}
				block.Cells = append(block.Cells, cell)
			}
		}
		out.Blocks = append(out.Blocks, block)
	}
	return out
}

// wireFromQuery converts a stored question back to the wire.
//
// The inverse of queryFromWire, and it exists because a saved run answers with
// the question it saved: a reader who wants to re-ask it, or ask a neighbouring
// one, needs it in the vocabulary they would have typed.
func wireFromQuery(in analyticsquery.Query) crmcontracts.AnalyticsQuery {
	out := crmcontracts.AnalyticsQuery{Entity: in.Entity}
	if len(in.GroupBy) > 0 {
		groupBy := in.GroupBy
		out.GroupBy = &groupBy
	}
	if in.Limit != 0 {
		limit := in.Limit
		out.Limit = &limit
	}
	for _, m := range in.Measures {
		measure := crmcontracts.AnalyticsMeasure{Fn: crmcontracts.AnalyticsMeasureFn(m.Fn)}
		if m.Field != "" {
			field := m.Field
			measure.Field = &field
		}
		if m.As != "" {
			as := m.As
			measure.As = &as
		}
		out.Measures = append(out.Measures, measure)
	}
	if len(in.Filters) > 0 {
		filters := make([]crmcontracts.AnalyticsFilter, 0, len(in.Filters))
		for _, f := range in.Filters {
			filters = append(filters, crmcontracts.AnalyticsFilter{
				Field: f.Field, Op: crmcontracts.AnalyticsFilterOp(f.Op), Value: f.Value,
			})
		}
		out.Filters = &filters
	}
	// Save is NOT carried back. It is an instruction about this call, not a
	// property of the question — echoing it would describe a saved run as one
	// that asks to be saved again.
	return out
}

// ExplainAnalyticsCell implements POST /analytics/explain.
func (h analyticsQueryHandlers) ExplainAnalyticsCell(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.AnalyticsExplainRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	in := analyticsquery.Explain{Query: queryFromWire(body.Query)}
	if body.Group != nil {
		in.Group = *body.Group
	}

	ctx := r.Context()
	var out AnalyticsExplanation
	if err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = ExplainAnalyticsCell(ctx, tx, in, h.floor)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AnalyticsExplanation{
		Columns: out.Columns, Rows: out.Rows,
		Withheld: out.Withheld, Truncated: out.Truncated,
	})
}
