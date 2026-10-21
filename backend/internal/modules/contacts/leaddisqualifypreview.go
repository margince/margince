// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func disqualifyLeadPatch(current crmcontracts.Lead, in DisqualifyLeadInput) *storekit.Patch {
	patch := storekit.NewPatch()
	patch.Set(leadStatusColumn, current.Status, crmcontracts.LeadStatusDisqualified)
	patch.Set("disqualify_reason_id", current.DisqualifyReasonId, in.ReasonID)
	patch.Set("disqualify_note", current.DisqualifyNote, in.Note)
	return patch
}

// PreviewDisqualifyLead shares the writer's reason validation and audit projection.
func (s *Store) PreviewDisqualifyLead(ctx context.Context, id ids.LeadID, in DisqualifyLeadInput, version int64) (*storekit.Patch, error) {
	if err := auth.Require(ctx, entityLead, principal.ActionDelete); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, entityLead, principal.ActionRead); err != nil {
		return nil, err
	}
	current, err := s.leadWrite(ctx, id, func(tx pgx.Tx, active []fieldcatalog.Column) (crmcontracts.Lead, error) {
		return readDisqualifiableLead(ctx, tx, id, in, writeOptions{atVersion: &version}, active)
	})
	if err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	for _, field := range auth.MaskedFields(actor, entityLead, current.Writable != nil && *current.Writable) {
		switch field {
		case leadStatusColumn, "disqualify_reason_id", "disqualify_note":
			return nil, apperrors.ErrPermissionDenied
		}
	}
	return disqualifyLeadPatch(current, in), nil
}

func readDisqualifiableLead(ctx context.Context, tx pgx.Tx, id ids.LeadID, in DisqualifyLeadInput, options writeOptions, active []fieldcatalog.Column) (crmcontracts.Lead, error) {
	var empty crmcontracts.Lead
	if _, err := storekit.LockRow(ctx, tx, entityLead, id.UUID, storekit.LiveOnly); err != nil {
		return empty, err
	}
	if err := refuseIfHumanTouched(ctx, tx, entityLead, id.UUID, options); err != nil {
		return empty, err
	}
	current, err := readLead(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return empty, err
	}
	if err := refuseIfVersionMoved(entityLead, current.Version, options); err != nil {
		return empty, err
	}
	if err := ensureDisqualifyReasonIfNamed(ctx, tx, in.ReasonID); err != nil {
		return empty, err
	}
	return current, nil
}
