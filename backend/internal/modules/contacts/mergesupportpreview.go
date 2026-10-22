// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeEmploymentSupport keeps employment evidence visible before a contact merge.
type MergeEmploymentSupport struct {
	Source          crmcontracts.Relationship
	Target          crmcontracts.Relationship
	Resolutions     int64
	AppliedFields   int64
	CollapsedFields int64
}

func (s *Store) previewMergeEmploymentSupport(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]MergeEmploymentSupport, error) {
	rows, err := tx.Query(ctx, `SELECT d.old_id, d.new_id,
 (SELECT count(*) FROM provider_employment_resolution e WHERE e.relationship_id=d.old_id),
 (SELECT count(*) FROM provider_applied_field f WHERE f.target_table='relationship' AND f.target_row_id=d.old_id),
 (SELECT count(*) FROM provider_applied_field f WHERE f.target_table='relationship' AND f.target_row_id=d.old_id AND `+mergeSupportAlreadyKept+`)
 FROM (`+mergeEmploymentDuplicates(kind == contactEntity)+`) d ORDER BY d.old_id LIMIT 501`, pgx.StrictNamedArgs{mergeSourceKey: source, mergeTargetKey: target})
	if err != nil {
		return nil, err
	}
	type transfer struct {
		Source, Target                 ids.UUID
		Resolutions, Fields, Collapsed int64
	}
	transfers, err := pgx.CollectRows(rows, pgx.RowToStructByPos[transfer])
	if err != nil {
		return nil, err
	}
	if len(transfers) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	var out []MergeEmploymentSupport
	for _, transfer := range transfers {
		if transfer.Resolutions == 0 && transfer.Fields == 0 {
			continue
		}
		from, err := s.visibleRelationship(ctx, tx, transfer.Source)
		if err != nil {
			return nil, err
		}
		to, err := s.visibleRelationship(ctx, tx, transfer.Target)
		if err != nil {
			return nil, err
		}
		for _, edge := range []relationshipRow{from, to} {
			if edge.ContactID == nil {
				return nil, apperrors.ErrInvalidArgument
			}
			if err := requireEmploymentRead(ctx, tx, *edge.ContactID); err != nil {
				return nil, err
			}
		}
		out = append(out, MergeEmploymentSupport{Source: wireRelationship(from), Target: wireRelationship(to), Resolutions: transfer.Resolutions, AppliedFields: transfer.Fields, CollapsedFields: transfer.Collapsed})
	}
	return out, nil
}
