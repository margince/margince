// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A request reminder's reach is derived from the mail it was read out of, and
// re-derived when that mail's reach changes.
//
// The reminder copies the answer once, when it is filed. A mail narrowed
// afterwards then hides the mail from the waiting lane. An opened one is read
// by its assignee alone.
//
// Neither leaks content, which the source's own gate decides. Both put two
// answers on one screen.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecomputeRequestReminderReachTx re-derives the audience of every open
// request reminder of the mail named by source.
//
// Called by the consumer that already answers an audience change, so a
// derivation and a member's own narrowing reach it the same way. An accepted
// request can leave a second open task on one mail, so it moves them all.
//
// Idempotent, and silent where there is no open reminder: the pass that calls
// it retries, and most messages have no reminder at all.
func RecomputeRequestReminderReachTx(ctx context.Context, tx pgx.Tx, source ids.ActivityID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	shared, err := readMailSharedWithWorkspace(ctx, tx, source.UUID)
	if err != nil {
		return err
	}
	reminders, err := openRemindersOf(ctx, tx, source)
	if err != nil {
		return err
	}
	for _, r := range reminders {
		want, move := reminderMove(shared, r.audience, r.assignee)
		if !move {
			continue
		}
		if err := writeReminderReachTx(ctx, tx, r.id, r.audience, want, r.assignee); err != nil {
			return err
		}
	}
	return nil
}

// reminderRow is one open reminder as the recompute reads it.
type reminderRow struct {
	id       ids.UUID
	audience string
	assignee *ids.UUID
}

// openRemindersOf locks the open reminders of one mail in id order. A
// reminder a human set the audience of carries ReasonManual, which no
// derivation moves.
func openRemindersOf(ctx context.Context, tx pgx.Tx, source ids.ActivityID) ([]reminderRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT t.id, t.audience, t.assignee_id FROM activity t
		 WHERE t.source_system = $1 AND t.source_activity_id = $2
		   AND t.audience_reason IS DISTINCT FROM $3
		   AND `+openReminderClause("t")+`
		 ORDER BY t.id
		 FOR UPDATE OF t`, EmailRequestTaskSource, source.UUID, ReasonManual)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the reminders of %s: %w", source, err)
	}
	reminders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (reminderRow, error) {
		var r reminderRow
		err := row.Scan(&r.id, &r.audience, &r.assignee)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading the reminders of %s: %w", source, err)
	}
	return reminders, nil
}

// writeReminderReachTx moves the reminder to want, with the membership that
// audience implies: the assignee alone for a narrowed one, nobody for the
// workspace's.
//
// The audience pin re-states what the caller read under FOR UPDATE, so a
// concurrent move leaves this a no-op rather than a second answer.
func writeReminderReachTx(
	ctx context.Context, tx pgx.Tx, reminder ids.UUID, stored, want string, assignee *ids.UUID,
) error {
	// archived_at as well as the audience pin. An archived reminder is off every
	// lane already, so moving its reach would write an audit row for a screen
	// nobody reads.
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET audience = $2
		 WHERE id = $1 AND audience = $3 AND restricted_at IS NULL
		   AND archived_at IS NULL AND audience_reason IS DISTINCT FROM $4`, reminder, want, stored, ReasonManual)
	if err != nil {
		return fmt.Errorf("activities: moving the reminder %s to %s: %w", reminder, want, err)
	}
	if tag.RowsAffected() != 1 {
		return nil
	}
	members := reminderMembers(want, assignee)
	if err := replaceAudienceMembers(ctx, tx, ids.From[ids.ActivityKind](reminder), members); err != nil {
		return err
	}
	before := map[string]any{auditFieldAudience: stored}
	after := map[string]any{auditFieldAudience: want}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", reminder, before, after)
	if err != nil {
		return err
	}
	// The shape SetAudience and the derivation both emit, so the consumers
	// downstream cannot tell a re-derivation from a member's own narrowing.
	audience := crmcontracts.PublicEventActivityChangedFieldsAudience(want)
	return storekit.EmitEvent(ctx, tx, auditID, reminder, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Audience: &audience},
	})
}

// reminderMove is the reach a reminder should carry for its mail, and whether
// to move it there.
//
// A reminder already agreeing with its mail is left alone, so a recompute that
// changes nothing writes no audit row. An unassigned reminder is not narrowed:
// a narrowed reach names its assignee as the one reader, and with none it
// would reach nobody. The source's own gate still decides its content.
func reminderMove(mailShared bool, stored string, assignee *ids.UUID) (string, bool) {
	want := reminderReach(mailShared)
	if want == string(crmcontracts.ActivityAudienceSelected) && assignee == nil {
		return stored, false
	}
	return want, want != stored
}

// reminderReach is the audience a reminder carries for a mail that is, or is
// not, the whole workspace's.
//
// Separate from the statements that read and write it, so the rule itself is
// provable without a database.
func reminderReach(mailShared bool) string {
	if mailShared {
		return string(crmcontracts.ActivityAudienceWorkspace)
	}
	return string(crmcontracts.ActivityAudienceSelected)
}

// reminderMembers is the membership that reach implies: the assignee alone for
// a narrowed reminder, nobody for the workspace's. reminderMove never narrows
// an unassigned reminder, so a narrowed one always names its reader.
func reminderMembers(reach string, assignee *ids.UUID) []AudienceMember {
	members := []AudienceMember{}
	if reach == string(crmcontracts.ActivityAudienceSelected) && assignee != nil {
		members = append(members, AudienceMember{
			SubjectType: string(crmcontracts.AudienceMemberSubjectTypeUser), SubjectID: *assignee,
		})
	}
	return members
}

// openReminderClause is the reminder this pass may still move, over the
// activity aliased by alias.
//
// Three rows are left alone, each for its own reason. A done reminder has
// been answered. An archived one is off every lane. A restricted one is under
// a statutory obligation, and its trigger refuses the write anyway.
func openReminderClause(alias string) string {
	a := pgx.Identifier{alias}.Sanitize()
	return a + `.kind = 'task' AND ` + a + `.archived_at IS NULL` +
		` AND NOT ` + a + `.is_done AND ` + a + `.restricted_at IS NULL`
}
