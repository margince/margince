// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A technical lookup's receipt, judged and reversed through the contacts
// module's own inverse. The lookup changes company_fact rows and no company
// column, so the field replay has no image to send. The module keeps the
// rows and the rule for when they may come back.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/auditverb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func isTechnicalReceipt(row AuditRow) bool {
	return row.EntityType == entityTypeCompany && row.Action == string(auditverb.Update) &&
		contacts.IsTechnicalReceipt(row.Evidence)
}

// evaluateTechnical runs the record refusals every reversal shares, then the
// module's own judgment. The preview and the write both come here; the write
// holds the company first.
func (e Evaluator) evaluateTechnical(ctx context.Context, tx pgx.Tx, row AuditRow) (Undoability, error) {
	if answer, decided, err := e.liveState(ctx, tx, row, nil); err != nil || decided {
		return answer, err
	}
	if e.AlreadyUndone != nil {
		undone, err := e.AlreadyUndone(ctx, tx, row)
		if err != nil {
			return Undoability{}, err
		}
		if undone {
			return refuse(ReasonAlreadyUndone, ""), nil
		}
	}
	err := contacts.JudgeTechnicalRestoreTx(ctx, tx, ids.From[ids.CompanyKind](row.EntityID), row.ID, row.OccurredAt, row.Evidence)
	return technicalAnswer(err)
}

func technicalAnswer(err error) (Undoability, error) {
	switch {
	case err == nil:
		return undoable(), nil
	case errors.Is(err, contacts.ErrTechnicalReceiptLegacy):
		return refuse(ReasonNoRestoreSnapshot, ""), nil
	case errors.Is(err, contacts.ErrTechnicalLaterChange):
		return refuse(ReasonLaterCompanyChange, ""), nil
	}
	return Undoability{}, err
}

// reverseTechnical holds the company, asks the evaluator under the hold, and
// lets the module put the rows back, all in one transaction. A second Undo
// waits on the hold and then reads the first one's restore row. Nothing is
// judged before the hold: two statements read outside it can straddle the
// other Undo's commit and disagree about what happened.
func (s RestoreSeam) reverseTechnical(
	ctx context.Context, entityType string, id ids.UUID, row AuditRow, ifVersion int64,
) (privacy.RecordHistoryEntry, error) {
	company := ids.From[ids.CompanyKind](row.EntityID)
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.inverses.contacts.LockTechnicalRestoreTx(ctx, tx, company, ifVersion); err != nil {
			// An archived company refuses the hold; the evaluator names why.
			if refusal, refused := errors.AsType[RefusedRestore](s.bindingRefusal(ctx, tx, row)); refused {
				return refusal
			}
			return err
		}
		if err := s.bindingRefusal(ctx, tx, row); err != nil {
			return err
		}
		err := s.inverses.contacts.RestoreTechnicalFactsTx(ctx, tx, company, row.ID, row.OccurredAt, row.Evidence,
			map[string]any{undidAuditLogID: row.ID.String()})
		if answer, judged := technicalAnswer(err); judged == nil && !answer.Undoable {
			return RefusedRestore{Reason: answer.Reason}
		}
		return err
	})
	if err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	return s.readRestoreEntry(ctx, entityType, id, row.ID)
}

// bindingRefusal is the evaluator's binding answer as an error: nil when the
// entry can be put back.
func (s RestoreSeam) bindingRefusal(ctx context.Context, tx pgx.Tx, row AuditRow) error {
	answer, err := s.evaluator.Evaluate(ctx, tx, row, Binding)
	if err != nil {
		return err
	}
	if !answer.Undoable {
		return RefusedRestore{Reason: answer.Reason, Detail: answer.Detail}
	}
	return nil
}
