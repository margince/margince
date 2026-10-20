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

type MergeRelationship struct {
	Relationship  crmcontracts.Relationship
	Retired       bool
	WillBePrimary bool
}

func (s *Store) previewMergeRelationships(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]MergeRelationship, error) {
	sourceSQL := "(" + mergeWorksWithSource + ") OR (" + mergeContactEdgeSource + ")"
	retiredSQL := "CASE WHEN a.kind='works_with' THEN " + mergeWorksWithDuplicate + " ELSE " + mergeContactEdgeDuplicate + " END"
	primarySQL := "CASE WHEN a.kind='works_with' THEN a.is_current_primary ELSE " + mergeContactEdgePrimary + " END"
	if kind == "company" {
		sourceSQL = mergeCompanyEdgeSource
		retiredSQL = "(" + mergeCompanyEdgeSelf + ") OR (" + mergeCompanyEdgeDuplicate + ")"
		primarySQL = "CASE WHEN a.company_id=@source THEN " + mergeCompanyEdgePrimary + " ELSE a.is_current_primary END"
	}
	rows, err := tx.Query(ctx, "SELECT a.id, COALESCE(("+retiredSQL+"), false), "+primarySQL+" FROM relationship a WHERE "+sourceSQL+" ORDER BY a.id LIMIT 501", pgx.StrictNamedArgs{"source": source, "target": target})
	if err != nil {
		return nil, err
	}
	type outcome struct {
		ID               ids.UUID
		Retired, Primary bool
	}
	outcomes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[outcome])
	if err != nil {
		return nil, err
	}
	if len(outcomes) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	if len(outcomes) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := s.authorizeMergePrimaryWitnesses(ctx, tx, kind, source, target, sourceSQL, retiredSQL); err != nil {
		return nil, err
	}
	var out []MergeRelationship
	for _, outcome := range outcomes {
		edge, err := s.visibleRelationship(ctx, tx, outcome.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, MergeRelationship{Relationship: wireRelationship(edge), Retired: outcome.Retired, WillBePrimary: !outcome.Retired && outcome.Primary})
	}
	return out, nil
}

func (s *Store) authorizeMergePrimaryWitnesses(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID, sourceSQL, retiredSQL string) error {
	witness := mergeContactPrimaryWitness
	affected := "a.kind <> 'works_with'"
	if kind == "company" {
		witness, affected = mergeCompanyPrimaryWitness, "a.company_id = @source"
	}
	rows, err := tx.Query(ctx, "SELECT DISTINCT b.id FROM relationship a JOIN relationship b ON "+witness+" WHERE ("+sourceSQL+") AND "+affected+" AND a.is_current_primary AND NOT COALESCE(("+retiredSQL+"), false) ORDER BY b.id LIMIT 501", pgx.NamedArgs{"source": source, "target": target})
	if err != nil {
		return err
	}
	witnesses, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	if len(witnesses) > 500 {
		return apperrors.ErrInvalidArgument
	}
	for _, id := range witnesses {
		if _, err := s.visibleRelationship(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}
