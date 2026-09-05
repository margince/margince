// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// run_analytics_query's engine seam and the vocabulary document it names.
//
// The tool and POST /analytics/query are one engine: both decode the same wire
// shape, compile against the same per-caller derived schema, run under the
// same floor and save through the same run store — so the two transports
// cannot disagree about what a median over a team's week is.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// analyticsQueryToolRunner adapts the typed query engine to the tool seam:
// strict-decode the wire shape, run, save when asked, re-encode.
// The floor is DefaultFloor on both transports: a figure a person may not see
// is one a model asking on their behalf may not see either.
func analyticsQueryToolRunner(db *database.DB) agents.AnalyticsQueryRunner {
	floor := analyticsquery.DefaultFloor
	return func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		var body crmcontracts.AnalyticsQuery
		// STRICT for the reason the report runner's decode is: a key this
		// engine does not serve is refused by name, not dropped — a lenient
		// decode would answer a request for something else with the wrong
		// thing and no warning.
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		err := dec.Decode(&body)
		if err == nil && dec.More() {
			// Exactly one JSON value, the report runner's rule: a second value
			// after the object is a caller who thinks they sent something
			// this never read.
			err = errors.New("trailing content after the arguments")
		}
		if err != nil {
			return nil, httperr.Validation("arguments", "malformed_json",
				"the arguments are not the shape this tool takes: entity (string), "+
					"group_by ([string]), measures ([{fn, field, as}]), filters "+
					"([{field, op, value}]), limit, save, scope_kind, scope_id")
		}
		q := queryFromWire(body)

		var answer AnalyticsAnswer
		var runID *ids.UUID
		if err := db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			answer, err = RunAnalyticsQuery(ctx, tx, q, floor)
			if err != nil {
				return err
			}
			if body.Save == nil || !*body.Save {
				return nil
			}
			// In the SAME transaction that produced it, exactly as the HTTP
			// twin saves: a run id must never name a row that does not exist.
			id, err := SaveReportRun(ctx, tx, q, answer, floor)
			if err != nil {
				return err
			}
			runID = &id
			return nil
		}); err != nil {
			return nil, err
		}

		// Never null: a model reads null as "unknown" where an empty array
		// says "none matched".
		rows := make([]json.RawMessage, 0, len(answer.Rows))
		for _, row := range answer.Rows {
			encoded, err := json.Marshal(row)
			if err != nil {
				return nil, fmt.Errorf("compose: encoding an analytics row: %w", err)
			}
			rows = append(rows, encoded)
		}
		out := agents.AnalyticsQueryResult{
			Columns:  emptyIfNilStrings(answer.Columns),
			Rows:     rows,
			Withheld: answer.Withheld, TotalSafe: answer.TotalSafe,
			SchemaVersion: answer.SchemaVersion,
		}
		if runID != nil {
			s := runID.String()
			out.RunID = &s
		}
		body2, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("compose: encoding an analytics answer: %w", err)
		}
		return body2, nil
	}
}

func emptyIfNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// AnalyticsSchemaURI names the vocabulary document run_analytics_query points
// at instead of carrying: the populations, dimensions and measures this seat
// may ask about, derived per caller — a masked field is indistinguishable
// from one that does not exist.
const AnalyticsSchemaURI = "margince://schema/analytics"

// mimeTextPlain is the analytics vocabulary's content type: prose, because it
// reaches a model that pays per token and a nested object spends the budget
// on punctuation.
const mimeTextPlain = "text/plain"

// analyticsSchemaResource publishes that document.
type analyticsSchemaResource struct{}

// Resources advertises the one document this provider publishes.
func (analyticsSchemaResource) Resources(context.Context) []mcp.Resource {
	return []mcp.Resource{{
		URI:           AnalyticsSchemaURI,
		Name:          "analytics-schema",
		Title:         "Analytics query vocabulary",
		RequiredScope: principal.ScopeRead,
		MIMEType:      mimeTextPlain,
		Description: "The populations a run_analytics_query plan may name, each with its " +
			"group_by dimensions and its measures, derived for this seat. " +
			"run_analytics_query names this document instead of carrying it.",
	}}
}

// ReadResource composes the document under the caller's own derivation, so a
// masked field never appears in it.
func (analyticsSchemaResource) ReadResource(ctx context.Context, uri string) (mcp.ResourceContents, error) {
	if uri != AnalyticsSchemaURI {
		return mcp.ResourceContents{}, fmt.Errorf("compose: resource %q: %w", uri, apperrors.ErrNotFound)
	}
	schema := AnalyticsSchemaFor(ctx)
	var out strings.Builder
	out.WriteString("schema version " + schema.Version + "\n")
	out.WriteString("aggregates: " + strings.Join(analyticsquery.AggregateNames(), ", ") + "\n")
	out.WriteString("filter ops: " + strings.Join(analyticsquery.FilterOpNames(), ", ") + "\n\n")
	out.WriteString(DescribeAnalyticsSchema(schema))
	return mcp.ResourceContents{URI: uri, MIMEType: mimeTextPlain, Text: out.String()}, nil
}
