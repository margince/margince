// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What an erasure or anonymize does to the tables an extension owns. The unit
// declares the personal columns and the record each row belongs to. Clearing
// and the audit row are the core's, so a unit cannot forget either.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

// errExtensionRowHeld refuses an erasure or a clearing while a row an extension
// keeps for the record is under legal hold. A held record refuses the same way.
var errExtensionRowHeld = fmt.Errorf("a row an extension keeps for this record is under legal hold: %w", apperrors.ErrConflict)

// refuseHeldExtensionRows returns errExtensionRowHeld when any row that names
// one of the subjects is held.
func refuseHeldExtensionRows(
	ctx context.Context, tx pgx.Tx, registry extprivacy.Registry, subjects map[extdecl.RecordKind][]ids.UUID,
) error {
	held, err := registry.Held(ctx, tx, subjects)
	if err != nil {
		return err
	}
	if len(held) > 0 {
		return errExtensionRowHeld
	}
	return nil
}

// clearExtensionRows clears every extension row that names one of the subjects
// and writes an anonymize audit row and event per cleared row. Earlier audit
// rows of those rows fall behind the erasure boundary, so the compliance log
// stops serving their images. A held row refuses the whole clearing, before any
// row changes.
func clearExtensionRows(
	ctx context.Context, tx pgx.Tx, registry extprivacy.Registry, reason string, subjects map[extdecl.RecordKind][]ids.UUID,
) (int, error) {
	if err := refuseHeldExtensionRows(ctx, tx, registry, subjects); err != nil {
		return 0, err
	}
	cleared, err := registry.Clear(ctx, tx, subjects)
	if err != nil {
		return 0, err
	}
	for _, row := range cleared {
		auditID, err := storekit.AuditWithEvidence(ctx, tx, actionAnonymize, row.Table, row.ID, nil, nil,
			map[string]any{evidenceKeyReason: reason})
		if err != nil {
			return 0, err
		}
		payload := retentionAppliedPayload(crmcontracts.RetentionAppliedAnonymize, nil, &reason)
		if err := storekit.EmitEventForEntity(ctx, tx, auditID, row.Table, row.ID, payload); err != nil {
			return 0, err
		}
	}
	return len(cleared), nil
}

// exportExtensionRows serves the personal columns the units declared, for the
// rows that name the subject, a lead twin or one of the exported activities.
func exportExtensionRows(
	ctx context.Context, tx pgx.Tx, registry extprivacy.Registry, contact ids.ContactID, leads []ids.UUID, activities []map[string]any,
) ([]map[string]any, error) {
	activityIDs := make([]ids.UUID, 0, len(activities))
	for _, a := range activities {
		raw, ok := a["id"].(string)
		if !ok {
			return nil, fmt.Errorf("privacy: an exported activity carries an id of type %T, want the text readableValue renders", a["id"])
		}
		id, err := ids.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("privacy: an exported activity carries id %q: %w", raw, err)
		}
		activityIDs = append(activityIDs, id)
	}
	return registry.Export(ctx, tx, map[extdecl.RecordKind][]ids.UUID{
		extdecl.Contact: {contact.UUID}, extdecl.Lead: leads, extdecl.Activity: activityIDs,
	})
}
