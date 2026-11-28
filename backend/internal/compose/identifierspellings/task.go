// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package identifierspellings wires owning modules into the legacy-spelling correction.
package identifierspellings

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// Columns are declared by their owners, including the own-domain text key.
func Columns() []storekit.IdentifierColumn {
	return slices.Concat(activities.IdentifierSpellings(), capture.IdentifierSpellings(), consent.IdentifierSpellings(), contacts.IdentifierSpellings(), dealrooms.IdentifierSpellings(), identity.IdentifierSpellings())
}

// DataTask changes representations without choosing between colliding identities.
func DataTask() datatask.Task {
	steps := []datatask.Step{}
	for _, column := range Columns() {
		steps = append(steps, columnStep(column))
	}
	return datatask.Task{ID: "core/1795826348_identifier_spellings", Version: 1, Title: systemtext.Literal("datatask.title.core.1795826348_identifier_spellings"), Requires: []string{"core/1795830905"}, Steps: steps}
}

func columnStep(column storekit.IdentifierColumn) datatask.Step {
	return datatask.Step{
		Name: "fold_" + column.Table, Limit: 100, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			count, err := column.Count(ctx, q, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			page, err := column.Repair(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			out, err := datatask.AdvanceUUID(in, page.Scanned, page.Changed)
			if len(page.Review) > 0 {
				out.Action = &datatask.ActionNeeded{Reason: "identifier_spelling_review"}
				for _, id := range page.Review {
					out.Action.Records = append(out.Action.Records, datatask.RecordRef{Type: column.Object, ID: id})
				}
			}
			return out, err
		},
	}
}
