// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package livepipeline answers whether an installation has one live pipeline,
// so a reporting reading need not ask its reader to choose between one.
package livepipeline

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// liveQuery reads two live pipelines at most: two already means several. An
// archived pipeline is no choice a reader could make, so it is not counted.
const liveQuery = "SELECT id FROM pipeline WHERE archived_at IS NULL ORDER BY id LIMIT 2"

// Mark names the installation's only live pipeline on frame, and leaves it
// unnamed when there are several or the reader may not read pipelines.
func Mark(ctx context.Context, tx pgx.Tx, frame *crmcontracts.ReportingContext) error {
	if !auth.Allows(ctx, "pipeline", principal.ActionRead) {
		return nil
	}
	rows, err := tx.Query(ctx, liveQuery)
	if err != nil {
		return err
	}
	live, err := pgx.CollectRows(rows, pgx.RowTo[openapi_types.UUID])
	if err != nil {
		return err
	}
	if len(live) == 1 {
		frame.OnlyLivePipeline = &live[0]
	}
	return nil
}

// Stages is the pipeline whose stages a stage chart compares: the chosen one,
// or the only live one when the selection asks for every pipeline. Nil means
// the reader must choose.
func Stages(frame crmcontracts.ReportingContext) *openapi_types.UUID {
	if frame.PipelineId != nil {
		return frame.PipelineId
	}
	return frame.OnlyLivePipeline
}
