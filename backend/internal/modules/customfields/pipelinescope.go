// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

const (
	fieldPipelines = "pipeline_ids"

	// The pipeline-scope refusals: pipelines on a field of another object, and
	// an id that names no live pipeline.
	codePipelinesNotDeal = "only_on_deal_fields"
	codeUnknownPipeline  = "unknown_pipeline"
)

// checkPipelineScope refuses pipelines on a field of another object. Whether
// each id names a live pipeline needs the database, so livePipelines asks it.
func checkPipelineScope(object string, pipelines []ids.UUID) []FieldError {
	if len(pipelines) > 0 && object != string(fieldcatalog.TargetDeal) {
		return []FieldError{{Field: fieldPipelines, Code: codePipelinesNotDeal}}
	}
	return nil
}

// storedPipelines is the stored form of a pipeline list: sorted and without
// repeats, and nil, which binds as SQL NULL, for every pipeline.
func storedPipelines(pipelines []ids.UUID) []ids.UUID {
	if len(pipelines) == 0 {
		return nil
	}
	sorted := slices.SortedFunc(slices.Values(pipelines), func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
	return slices.Compact(sorted)
}

// livePipelines refuses an id that names no unarchived pipeline. An empty list
// is every pipeline and asks nothing. A pipeline archived later keeps its id in
// the list and matches nothing, because the column cannot carry a foreign key.
func livePipelines(ctx context.Context, tx pgx.Tx, pipelines []ids.UUID) error {
	if len(pipelines) == 0 {
		return nil
	}
	var live int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM pipeline WHERE id = ANY($1) AND archived_at IS NULL`, pipelines).Scan(&live); err != nil {
		return fmt.Errorf("customfields: checking pipelines: %w", err)
	}
	if live != len(pipelines) {
		return &ValidationError{Errors: []FieldError{{Field: fieldPipelines, Code: codeUnknownPipeline}}}
	}
	return nil
}
