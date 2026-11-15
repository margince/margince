// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) reverseRelationshipUpdate(ctx context.Context, in ReverseEdgeInput, patch UpdateRelationshipInput) error {
	if err := auth.Require(ctx, "relationship", principal.ActionUpdate); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := lockContactForEmployment(ctx, tx, in.EdgeID); err != nil {
			return err
		}
		if err := s.archiveEmploymentReplacementTx(ctx, tx, in); err != nil {
			return err
		}
		_, err := s.updateRelationshipInTx(ctx, tx, in.EdgeID, patch, by)
		return err
	})
}

// The replacement is removed only while unchanged; a human's later work wins over an undo.
func (s *Store) archiveEmploymentReplacementTx(ctx context.Context, tx pgx.Tx, in ReverseEdgeInput) error {
	raw, ok := in.Evidence[storekit.EvidenceKeyUndidAuditLog].(string)
	if !ok {
		return nil
	}
	audit, err := ids.Parse(raw)
	if err != nil {
		return err
	}
	var replacement ids.UUID
	var version int64
	err = tx.QueryRow(ctx, `SELECT replacement.id,replacement.version FROM audit_log entry
 JOIN provider_employment_resolution resolution ON resolution.claim_id::text=entry.evidence->>'replacement_claim'
 AND resolution.episode_key=entry.evidence->>'replacement_episode' AND resolution.state='linked'
 JOIN relationship replacement ON replacement.id=resolution.relationship_id AND replacement.archived_at IS NULL
 JOIN relationship original ON original.id=entry.entity_id AND original.kind='employment' AND replacement.kind='employment' AND original.contact_id=replacement.contact_id AND original.company_id=replacement.company_id
 WHERE entry.id=@audit AND entry.entity_type='relationship' AND entry.entity_id=@edge AND entry.action='update'
 FOR UPDATE OF replacement`, pgx.NamedArgs{"audit": audit, "edge": in.EdgeID}).Scan(&replacement, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := s.visibleRelationship(ctx, tx, replacement); err != nil {
		return err
	}
	if version != 1 {
		var humanEdit bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_log WHERE entity_type='relationship' AND entity_id=@id AND action<>'create' AND actor_type IN ('human','agent') AND evidence->>'source' IS DISTINCT FROM @source)`, pgx.NamedArgs{"id": replacement, auditKeySource: enrichSource}).Scan(&humanEdit); err != nil {
			return err
		}
		if humanEdit {
			return apperrors.ErrVersionSkew
		}
	}
	_, err = s.archiveRelationshipTx(ctx, tx, replacement, &version, in.Evidence)
	return err
}
