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

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
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
	var answer AnalyticsAnswer
	if err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		answer, err = RunAnalyticsQuery(ctx, tx, q, h.floor)
		return err
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AnalyticsAnswer{
		Columns: answer.Columns, Rows: answer.Rows,
		Withheld: answer.Withheld, TotalSafe: answer.TotalSafe,
		SchemaVersion: answer.SchemaVersion,
	})
}

// queryFromWire converts the request without validating it.
//
// Validation belongs to the compiler, which is the thing that renders SQL: a
// second check here would be a second answer to what is askable, and the two
// would drift the first time a measure was added.
func queryFromWire(in crmcontracts.AnalyticsQuery) analyticsquery.Query {
	out := analyticsquery.Query{Entity: in.Entity}
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
