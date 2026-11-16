// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

// eraseRequestReminders erases the reminder filed for a customer's request.
// It copies the mail's subject and quotes the sender's ask, so it is text of
// the message and goes with it. A held reminder keeps what its hold keeps.
// The erased mark is the system text, which no mail can carry as its subject.
func (e *Eraser) eraseRequestReminders(ctx context.Context, tx pgx.Tx, source ids.UUID, act erasureAct) error {
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity a
 WHERE a.source_system = @system AND a.source_activity_id = @source AND a.system_text IS DISTINCT FROM @erased
 AND a.restricted_at IS NULL AND NOT `+retentionscope.HeldThroughAnyLink("a.id")+`
 ORDER BY a.id FOR UPDATE`,
		pgx.NamedArgs{"system": provenance.EmailRequestSource, "source": source, "erased": erasedActivityText()})
	if err != nil {
		return err
	}
	reminders, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, reminder := range reminders {
		if err := e.eraseRequestReminder(ctx, tx, reminder, act); err != nil {
			return err
		}
	}
	return nil
}

func (e *Eraser) eraseRequestReminder(ctx context.Context, tx pgx.Tx, reminder ids.UUID, act erasureAct) error {
	if err := clearActivityContent(ctx, tx, reminder); err != nil {
		return err
	}
	if err := e.purgeContentDerivedFrom(ctx, tx, reminder, act); err != nil {
		return err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "activity", reminder, nil, nil,
		map[string]any{evidenceKeyReason: act.collateralReason, evidenceKeyCause: act.cause})
	if err != nil {
		return err
	}
	if err := storekit.RedactAuditHistory(ctx, tx, auditID); err != nil {
		return err
	}
	return storekit.EmitEventForEntity(ctx, tx, auditID, "activity", reminder,
		retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, &act.collateralReason))
}
