// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// compose_analytics_report on the tool surface, answered by the SAME validator
// and renderer the HTTP surface uses.
//
// One engine, two transports. The tool decodes its own arguments — a model
// sends a document, not an http.Request — and everything after that is
// RenderReport, so the two surfaces cannot come to disagree about what a report
// may say or what a figure resolves to.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// analyticsReportComposer renders a composed document for the tool surface.
//
// The floor is the installation's, taken here the way the HTTP handler takes
// it: a figure a person may not see is a figure a model asking on their behalf
// may not see either, and a tool that floored differently would be a second
// answer to what a reader is allowed to be told.
func analyticsReportComposer(pool *pgxpool.Pool, floor analyticsquery.Floor) agents.AnalyticsReportComposer {
	store := forecasting.NewStore(InstallationDB(pool))
	return func(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
		var doc reportdoc.Document
		if err := json.Unmarshal(in, &doc); err != nil {
			// A malformed payload is the caller's, so it unwraps to the same
			// sentinel a malformed block does rather than reading as a server
			// fault the caller cannot act on.
			return nil, &reportdoc.InvalidError{
				Where:  "document",
				Reason: "is not a report document",
			}
		}

		var blocks []RenderedBlock
		if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			blocks, err = RenderReport(ctx, tx, doc, floor)
			return err
		}); err != nil {
			return nil, err
		}

		// Empty, never nil: a document that rendered no blocks cannot happen —
		// Validate refuses one — but a nil here would marshal to null, which a
		// model reads as "unknown" rather than as "none".
		if blocks == nil {
			blocks = []RenderedBlock{}
		}
		encoded, err := json.Marshal(blocks)
		if err != nil {
			return nil, fmt.Errorf("compose: rendering a composed report: %w", err)
		}
		return json.Marshal(agents.ComposeAnalyticsReportResult{Blocks: encoded})
	}
}
