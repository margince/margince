// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// agedReminderBatch bounds one pass, like the minting pass beside it.
const agedReminderBatch = 64

// retireAgedReminders archives the reminders this pass filed whose request has
// passed the waiting horizon, unless a human worked them. Archiving rather than
// completing: an aged request is not an answered one. heldRequestSQL is the
// other side of the same rule: a reminder a human wrote keeps its request.
func (s *Store) retireAgedReminders(ctx context.Context, tx pgx.Tx, asOf time.Time) error {
	cutoff, err := requestReminderCutoff(ctx, tx, asOf)
	if err != nil {
		return err
	}
	return retirePristineRemindersTx(ctx, tx, asOf, "a.occurred_at < @cutoff -- occurred_at raw: the age of a captured mail, an instant\n",
		pgx.NamedArgs{"cutoff": cutoff}, "past the waiting horizon")
}

// retireNeverOwedReminders archives the reminders this pass filed for a
// calendar invitation no verdict can make owed: a colleague's, or one whose
// meeting has begun. It settles reminders filed before that rule existed too.
// An invitation from one of our own domains counts as a colleague's, as in the
// Worklist. The seat stamp is missing on older mail and on non-seat senders.
func (s *Store) retireNeverOwedReminders(ctx context.Context, tx pgx.Tx, asOf time.Time, ownDomains []string) error {
	never := `(` + invitationNeverOwedSQL("@asOf::timestamptz") + ` OR (` + calendarInvitationSQL + ` AND ` +
		ownDomainSenderAtSQL("a", "@ownDomains") + `))`
	return retirePristineRemindersTx(ctx, tx, asOf, never,
		pgx.NamedArgs{"asOf": asOf, "ownDomains": ownDomains}, "for an invitation")
}

// retirePristineRemindersTx archives this pass's own reminders whose request
// (alias a) matches, unless a human worked them. Both are asked again under the
// row lock, so an edit or new evidence that committed while it waited counts.
func retirePristineRemindersTx(ctx context.Context, tx pgx.Tx, asOf time.Time, request string, args pgx.NamedArgs, what string) error {
	args[paramRepairSource] = EmailRequestTaskSource
	args["actor"] = OwedVerdictCapturedBy
	args["limit"] = agedReminderBatch
	rows, err := tx.Query(ctx, `
		SELECT reminder.id FROM activity reminder
		  JOIN activity a ON a.id = reminder.source_activity_id
		 WHERE reminder.source_system = @source
		   AND NOT reminder.is_done AND reminder.archived_at IS NULL
		   AND reminder.captured_by = @actor
		   AND `+request+`
		   AND `+pristineRepairTaskSQL("reminder")+`
		 ORDER BY a.occurred_at, reminder.id
		 LIMIT @limit
		 FOR UPDATE OF reminder SKIP LOCKED`, args)
	if err != nil {
		return fmt.Errorf("activities: reading reminders %s: %w", what, err)
	}
	locked, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: reading reminders %s: %w", what, err)
	}
	args["locked"] = locked
	rows, err = tx.Query(ctx, `SELECT reminder.id FROM activity reminder
		  JOIN activity a ON a.id = reminder.source_activity_id
		 WHERE reminder.id = ANY(@locked) AND `+request+` AND `+pristineRepairTaskSQL("reminder"), args)
	if err != nil {
		return fmt.Errorf("activities: rechecking reminders %s under their lock: %w", what, err)
	}
	pristine, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: rechecking reminders %s under their lock: %w", what, err)
	}
	for _, id := range pristine {
		if _, err := archiveActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), nil, archiveOptions{}, asOf); err != nil {
			return fmt.Errorf("activities: retiring the reminder %s %s: %w", id, what, err)
		}
	}
	return nil
}
