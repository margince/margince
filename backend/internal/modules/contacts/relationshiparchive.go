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

// removalGrants are the relationship grants that admit taking an edge of kind
// off. A deal stakeholder comes off with the grant that put it on. So whoever
// may seat a contact on a deal they may write may also unseat them. Every other
// kind needs delete. frontend/src/components/relationships.edges.tsx mirrors it.
func removalGrants(kind string) []principal.Action {
	if kind == dealStakeholderKind {
		return []principal.Action{principal.ActionDelete, principal.ActionCreate}
	}
	return []principal.Action{principal.ActionDelete}
}

// requireAnyRemovalGrant refuses, before any read, a seat that may remove no
// kind of edge at all.
func requireAnyRemovalGrant(ctx context.Context) error {
	return auth.RequireAny(ctx, "relationship", removalGrants(dealStakeholderKind)...)
}

// requireRemovalGrant is the relationship grant for removing one stored edge
// of kind. The anchor's update grant and row authority are asked beside it.
func requireRemovalGrant(ctx context.Context, kind string) error {
	return auth.RequireAny(ctx, "relationship", removalGrants(kind)...)
}

func (s *Store) archiveRelationshipTx(ctx context.Context, tx pgx.Tx, id ids.UUID, ifVersion *int64, evidence map[string]any) (relationshipRow, error) {
	if err := requireAnyRemovalGrant(ctx); err != nil {
		return relationshipRow{}, err
	}
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
	if err := requireRemovalGrant(ctx, current.Kind); err != nil {
		return relationshipRow{}, err
	}
	anchorObject, _ := relationshipAnchor(current.Kind)
	if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
		return relationshipRow{}, err
	}
	if err := ensureRelationshipAnchorWritable(ctx, tx, current.Kind, current.endpoints()); err != nil {
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
