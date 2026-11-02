// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ThreadHoldForSeatTx reads only a seat that actually imported this message;
// a thread verdict cannot manufacture another seat's mailbox contribution.
func ThreadHoldForSeatTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID) (string, string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return "", "", err
	}
	var status, kind, ownStatus, ownReason, posture *string
	err := tx.QueryRow(ctx, `SELECT v.status,v.kind,ci.verdict_status,ci.verdict_reason,ci.posture_at_import
 FROM activity a JOIN capture_import ci ON ci.activity_id=a.id AND ci.user_id=@seat
 LEFT JOIN capture_thread_verdict v ON v.thread_key=a.thread_key AND v.user_id=ci.user_id
 WHERE a.id=@id AND a.archived_at IS NULL AND a.restricted_at IS NULL`, pgx.NamedArgs{"id": id, threadArgSeat: seat}).Scan(&status, &kind, &ownStatus, &ownReason, &posture)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	return threadHoldForContribution(status, kind, ownStatus, ownReason, posture)
}

func threadHoldForContribution(status, kind, ownStatus, ownReason, posture *string) (string, string, error) {
	inherited := ""
	if status != nil {
		inherited = *status
	}
	if inherited == VerdictPending && ownStatus == nil && heldImportPosture(posture) {
		return "", "", nil
	}
	if ownStatus != nil && holdsItsMessages(*ownStatus) && (!holdsItsMessages(inherited) || verdictHoldRank(*ownStatus) >= verdictHoldRank(inherited)) {
		why := audienceReasonInheritedVerdict
		if ownReason != nil && *ownReason != "" {
			why = *ownReason
		}
		return *ownStatus, why, nil
	}
	if !holdsItsMessages(inherited) {
		return "", "", nil
	}
	why := audienceReasonInheritedVerdict
	if kind != nil && *kind != "" {
		why = rowReasonForKind(*kind)
	}
	return inherited, why, nil
}

func heldImportPosture(posture *string) bool {
	return posture != nil && (*posture == PostureHeld || *posture == PostureClassified)
}

// ThreadHoldImportTx upgrades one contribution after the activity owner holds
// its row lock. A firmer existing hold always survives a later repair pass.
func ThreadHoldImportTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID, status, reason string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if !holdsItsMessages(status) {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE capture_import SET verdict_status=@status,verdict_reason=CASE WHEN verdict_reason=ANY(@independent::text[]) THEN verdict_reason ELSE @reason END
 WHERE activity_id=@id AND user_id=@seat
 AND (verdict_status IS NULL OR verdict_status=ANY(@weaker::text[]))`, pgx.NamedArgs{"id": id, threadArgSeat: seat, columnStatus: status, fieldReason: reason, "weaker": weakerThreadVerdicts(status), "independent": verdictProofReasons})
	return err
}
