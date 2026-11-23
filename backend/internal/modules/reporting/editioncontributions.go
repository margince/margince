// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func writeEditionContributions(ctx context.Context, tx pgx.Tx, edition openapi_types.UUID, facts []Fact) error {
	// Every fact is judged before the first statement. An edition with one unknown
	// source is refused whole, not after its first chunk was written.
	for _, fact := range facts {
		if err := requireKnownSourceType(fact.SourceType); err != nil {
			return err
		}
	}
	// Bound both parameter count and encoded payload per database round trip.
	for start := 0; start < len(facts); start += 250 {
		var b bindings
		tuples := []string{}
		for _, fact := range facts[start:min(start+250, len(facts))] {
			raw, err := encode(fact)
			if err != nil {
				return err
			}
			values := []string{b.add(edition), b.add(fact.Metric), b.add(fact.ContextID), b.add(fact.Row.Key), b.add(fact.SourceType), b.add(fact.SourceID), b.add(raw)}
			tuples = append(tuples, "("+strings.Join(values, ",")+")")
		}
		if _, err := tx.Exec(ctx, "INSERT INTO report_edition_contribution(edition_id,metric,context_id,contribution_key,source_type,source_id,fact) VALUES "+strings.Join(tuples, ","), b.values...); err != nil {
			return err
		}
	}
	return nil
}
