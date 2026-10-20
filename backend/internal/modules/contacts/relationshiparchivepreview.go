// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type RelationshipArchivePreview struct {
	Relationship crmcontracts.Relationship
	Successor    *crmcontracts.Relationship
}

func (s *Store) PreviewRelationshipArchive(ctx context.Context, id ids.UUID, version *int64) (RelationshipArchivePreview, error) {
	var out RelationshipArchivePreview
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		return out, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		current, err := s.visibleRelationship(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.ArchivedAt != nil {
			return apperrors.ErrNotFound
		}
		if err := refuseGenericProjectCompany(current.Kind); err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, "relationship", id, version); err != nil {
			return err
		}
		out.Relationship = wireRelationship(current)
		if current.Kind != employmentKind || current.ContactID == nil {
			return nil
		}
		var successor ids.UUID
		err = tx.QueryRow(ctx, employmentSuccessorSQL(), pgx.StrictNamedArgs{"contact": *current.ContactID, "excluded": id}).Scan(&successor)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		remaining, err := s.visibleRelationship(ctx, tx, successor)
		if err != nil {
			return err
		}
		out.Successor = new(wireRelationship(remaining))
		return nil
	})
	if err != nil {
		return RelationshipArchivePreview{}, err
	}
	return out, nil
}
