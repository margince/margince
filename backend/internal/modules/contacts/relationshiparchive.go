// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) archiveRelationshipTx(ctx context.Context, tx pgx.Tx, id ids.UUID, ifVersion *int64, evidence map[string]any) (relationshipRow, error) {
	var out relationshipRow
	var err error

	// Employment writers take the contact lock before touching either edge.
	if err := lockContactForEmployment(ctx, tx, id); err != nil {
		return relationshipRow{}, err
	}
	current, err := s.visibleRelationship(ctx, tx, id)
	if err != nil {
		return relationshipRow{}, err
	}
	if err := refuseGenericProjectCompany(current.Kind); err != nil {
		return relationshipRow{}, err
	}
	if err := requireRelationshipArchive(ctx, tx, current); err != nil {
		return relationshipRow{}, err
	}
	p := storekit.NewPatch()
	p.Set("archived_at", nil, time.Now().UTC())
	if err := p.ApplyGuarded(ctx, tx, "relationship", id, ifVersion); err != nil {
		return relationshipRow{}, err
	}
	row := tx.QueryRow(ctx, `SELECT `+relationshipColumns+` FROM relationship WHERE id = $1`, id)
	if out, err = scanRelationship(row); errors.Is(err, pgx.ErrNoRows) {
		return relationshipRow{}, apperrors.ErrNotFound
	} else if err != nil {
		return relationshipRow{}, err
	}
	if err := emitRelationshipChangeWithEvidence(ctx, tx, "archive", nil, out, evidence); err != nil {
		return relationshipRow{}, err
	}
	if out.Kind != employmentKind || out.ContactID == nil {
		return out, nil
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return relationshipRow{}, err
	}
	return out, promoteThenFileEmployerTwins(ctx, tx, *out.ContactID, capturedBy)
}

// requireRelationshipArchive asks the authority questions of an edge removal,
// shared by the archive and its stage-time refusal. A deal stakeholder seat is
// removed by whoever may write its deal, as it is added; every other kind also
// needs the relationship delete grant.
func requireRelationshipArchive(ctx context.Context, tx pgx.Tx, current relationshipRow) error {
	if current.Kind != dealStakeholderKind {
		if err := auth.Require(ctx, "relationship", principal.ActionDelete); err != nil {
			return err
		}
	}
	anchorObject, _ := relationshipAnchor(current.Kind)
	if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
		return err
	}
	return ensureRelationshipAnchorWritable(ctx, tx, current.Kind, current.endpoints())
}
