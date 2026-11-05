// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ColleagueStamp is one received copy that capture read as a colleague's sent
// mail, with the colleague's login address as it stands now.
type ColleagueStamp struct {
	ID        ids.UUID
	Colleague ids.UUID
	Address   string
}

// colleagueStampScope selects the stamped copies the sender's own mailbox has
// not attested. An attested copy was filed as sent by that mailbox, so it is
// proven another way and stays. A held copy is read by nobody, so its stamp
// opens nothing.
const colleagueStampScope = `FROM activity a JOIN app_user u ON u.id = a.sent_by_colleague
	 WHERE a.sent_by_colleague IS NOT NULL AND NOT a.counterparty_outbound_attested AND a.restricted_at IS NULL`

// ColleagueStampsTx locks the next limit stamped copies after the cursor, in id order.
func ColleagueStampsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ColleagueStamp, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.sent_by_colleague, u.email `+colleagueStampScope+`
		   AND a.id > $1 ORDER BY a.id LIMIT $2 FOR UPDATE OF a`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: reading copies read as a colleague's mail: %w", err)
	}
	stamps, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ColleagueStamp])
	if err != nil {
		return nil, fmt.Errorf("activities: reading copies read as a colleague's mail: %w", err)
	}
	return stamps, nil
}

// CountColleagueStamps counts the stamped copies, reading at most maximum+1.
func CountColleagueStamps(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+colleagueStampScope+` LIMIT $1) stamped`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting copies read as a colleague's mail: %w", err)
	}
	return count, nil
}

// TurnBackColleagueCopyTx reads a stamped copy as received mail from the
// colleague's address again. What the stamp made true goes with it. That is the
// colleague as its sender and a settlement verdict judged through it. It is
// also the reminder that verdict closed when no human touched it. A copy no longer
// stamped for that colleague is left alone, so a replay changes nothing.
func TurnBackColleagueCopyTx(ctx context.Context, tx pgx.Tx, stamp ColleagueStamp) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	// An archived copy is turned back too: unarchived, it would open again.
	if _, err := storekit.LockRow(ctx, tx, "activity", stamp.ID, storekit.IncludeArchived); err != nil {
		return false, err
	}
	from := correspondence.Fold(stamp.Address)
	var wasCounterparty *string
	err := tx.QueryRow(ctx, `
		UPDATE activity a
		   SET direction = 'inbound', counterparty_email = $3, sent_by_colleague = NULL
		  FROM (SELECT counterparty_email FROM activity WHERE id = $1) was
		 WHERE a.id = $1 AND a.sent_by_colleague = $2 AND NOT a.counterparty_outbound_attested AND a.restricted_at IS NULL
		RETURNING was.counterparty_email`, stamp.ID, stamp.Colleague, from).Scan(&wasCounterparty)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading %s as received mail again: %w", stamp.ID, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM activity_participant
		 WHERE activity_id = $1 AND role = 'from' AND user_id = $2`, stamp.ID, stamp.Colleague); err != nil {
		return false, fmt.Errorf("activities: removing the colleague as sender of %s: %w", stamp.ID, err)
	}
	reopened, err := unjudgeThroughTx(ctx, tx, stamp.ID)
	if err != nil {
		return false, err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", stamp.ID,
		map[string]any{fieldDirection: string(crmcontracts.ActivityDirectionOutbound), auditFieldCounterparty: wasCounterparty, auditFieldSentByColleague: stamp.Colleague},
		map[string]any{fieldDirection: string(crmcontracts.ActivityDirectionInbound), auditFieldCounterparty: from, auditFieldSentByColleague: nil},
		map[string]any{"reopened_reminders": reopened})
	if err != nil {
		return false, err
	}
	became := crmcontracts.DirectionBecameInbound
	if err := storekit.EmitEvent(ctx, tx, auditID, stamp.ID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Direction: &became},
	}); err != nil {
		return false, err
	}
	return true, nil
}

const (
	auditFieldCounterparty    = "counterparty_email"
	auditFieldSentByColleague = "sent_by_colleague"
)

// unjudgeThroughTx removes the settlement verdicts judged through a copy and
// reopens each reminder a "settled" verdict closed, unless a human worked it.
func unjudgeThroughTx(ctx context.Context, tx pgx.Tx, copyID ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		WITH unjudged AS (
		  DELETE FROM activity_request_settlement s WHERE s.judged_through_activity_id = $1
		  RETURNING s.request_activity_id, s.verdict
		)
		UPDATE activity r SET is_done = false, done_at = NULL
		  FROM unjudged
		 WHERE unjudged.verdict = 'settled'
		   AND r.source_system = $2 AND r.source_activity_id = unjudged.request_activity_id
		   AND r.is_done AND r.archived_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM audit_log worked
		         WHERE worked.entity_type = 'activity' AND worked.entity_id = r.id AND worked.actor_type = $3)
		RETURNING r.id`, copyID, EmailRequestTaskSource, string(principal.PrincipalHuman))
	if err != nil {
		return nil, fmt.Errorf("activities: removing verdicts judged through %s: %w", copyID, err)
	}
	reopened, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: removing verdicts judged through %s: %w", copyID, err)
	}
	open := false
	for _, reminder := range reopened {
		auditID, err := storekit.Audit(ctx, tx, "update", "activity", reminder,
			map[string]any{"is_done": true}, map[string]any{"is_done": false})
		if err != nil {
			return nil, err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, reminder, crmcontracts.PublicEventActivityUpdated{
			ChangedFields: crmcontracts.PublicEventActivityChangedFields{IsDone: &open},
		}); err != nil {
			return nil, err
		}
	}
	return reopened, nil
}
