// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

const qualifyingCarryDuplicate = `a.source_entity_id IS NOT NULL AND EXISTS (
 SELECT 1 FROM consent_qualifying_event b WHERE b.contact_id=@target
 AND b.source_entity_type=a.source_entity_type AND b.source_entity_id=a.source_entity_id)`

func previewCarriedQualifyingEvents(ctx context.Context, tx pgx.Tx, source, target ids.ContactID) ([]commsauthz.CarriedQualifyingEvent, error) {
	rows, err := tx.Query(ctx, `SELECT a.id,a.kind,a.source_entity_type,a.source_entity_id,a.note,a.occurred_at,
 `+qualifyingCarryDuplicate+` FROM consent_qualifying_event a WHERE a.contact_id=@source ORDER BY a.id LIMIT 501`, pgx.NamedArgs{"source": source, "target": target})
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commsauthz.CarriedQualifyingEvent, error) {
		var event commsauthz.CarriedQualifyingEvent
		err := row.Scan(&event.ID, &event.Kind, &event.SourceType, &event.SourceID, &event.Note, &event.OccurredAt, &event.Collapsed)
		return event, err
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	for _, event := range out {
		if event.SourceID == nil && event.SourceType == nil {
			continue
		}
		if event.SourceID == nil || event.SourceType == nil {
			return nil, apperrors.ErrInvalidArgument
		}
		if err := authorizeCarryEvidence(ctx, tx, *event.SourceType, *event.SourceID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func authorizeCarryEvidence(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) error {
	if kind != "activity" && kind != "deal" {
		return apperrors.ErrInvalidArgument
	}
	if kind == "activity" {
		if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
			return err
		}
		return auth.EnsureActivityContentVisible(ctx, tx, id)
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	return auth.EnsureVisibleExisting(ctx, tx, "deal", id)
}
