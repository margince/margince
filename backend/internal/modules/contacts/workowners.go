// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WorkOwnership is who owns a record that can still carry work, and the row
// version an assignment must match.
type WorkOwnership struct {
	OwnerID *ids.UUID
	Version int64
}

// WorkOwners reads visible live records that can still carry current work.
func (s *Store) WorkOwners(ctx context.Context, kind string, recordIDs []ids.UUID) (map[ids.UUID]WorkOwnership, error) {
	var table string
	switch kind {
	case leadEntity:
		table = leadEntity
	case contactEntity:
		table = contactEntity
	default:
		return nil, apperrors.ErrInvalidArgument
	}
	if err := auth.Require(ctx, table, principal.ActionRead); err != nil {
		return nil, err
	}
	if len(recordIDs) > 200 {
		return nil, apperrors.ErrInvalidArgument
	}
	out := map[ids.UUID]WorkOwnership{}
	if len(recordIDs) == 0 {
		return out, nil
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		visible, err := auth.VisibleSubset(ctx, tx, table, recordIDs)
		if err != nil {
			return err
		}
		allowed := make([]ids.UUID, 0, len(recordIDs))
		for _, id := range recordIDs {
			if visible[id] {
				allowed = append(allowed, id)
			}
		}
		if len(allowed) == 0 {
			return nil
		}
		query := `SELECT id,owner_id,version FROM ` + pgx.Identifier{table}.Sanitize() + ` WHERE id=ANY(@ids) AND archived_at IS NULL`
		args := pgx.NamedArgs{"ids": allowed}
		if table == leadEntity {
			query += ` AND status<>ALL(@closed)`
			args["closed"] = []string{string(crmcontracts.LeadStatusPromoted), string(crmcontracts.LeadStatusDisqualified)}
		}
		rows, err := tx.Query(ctx, query, args)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			var owner WorkOwnership
			if err := rows.Scan(&id, &owner.OwnerID, &owner.Version); err != nil {
				return err
			}
			out[id] = owner
		}
		return rows.Err()
	})
	return out, err
}
