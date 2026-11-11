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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

// clearExtensionRows clears every extension row that names one of the subjects
// and writes an anonymize audit row and event per cleared row. The audit row is
// the certificate that strips the stored images of the row's earlier entries.
func clearExtensionRows(
	ctx context.Context, tx pgx.Tx, registry extprivacy.Registry, reason, cause string, subjects map[extdecl.RecordKind][]ids.UUID,
) error {
	cleared, err := registry.Clear(ctx, tx, subjects)
	if err != nil {
		return err
	}
	for _, row := range cleared {
		auditID, err := storekit.AuditWithEvidence(ctx, tx, actionAnonymize, row.Table, row.ID, nil, nil,
			map[string]any{evidenceKeyReason: reason, evidenceKeyCause: cause})
		if err != nil {
			return err
		}
		if err := storekit.RedactAuditHistory(ctx, tx, auditID); err != nil {
			return err
		}
		payload := retentionAppliedPayload(crmcontracts.RetentionAppliedAnonymize, nil, &reason)
		if err := storekit.EmitEventForEntity(ctx, tx, auditID, row.Table, row.ID, payload); err != nil {
			return err
		}
	}
	return nil
}

// scrubCopiesOutsideCore clears what other parties keep of the erased subject:
// the webhook bodies rendered from them and the declared extension columns.
func (e *Eraser) scrubCopiesOutsideCore(
	ctx context.Context, tx pgx.Tx, subject ids.ContactID, reason string, leads, activities []ids.UUID,
) error {
	if err := e.scrubDeliveryCopies(ctx, tx, subject, leads); err != nil {
		return err
	}
	return clearExtensionRows(ctx, tx, e.extensions, reason, causeContactErasure, map[extdecl.RecordKind][]ids.UUID{
		extdecl.Contact: {subject.UUID}, extdecl.Lead: leads, extdecl.Activity: activities,
	})
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
