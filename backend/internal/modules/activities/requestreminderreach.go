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

// RecomputeRequestReminderReachTx re-derives the open request reminder's
// audience from the mail named by source.
//
// Called by the consumer that already answers an audience change, so a
// derivation and a member's own narrowing reach it the same way.
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
	var reminder ids.UUID
	var stored string
	var assignee *ids.UUID
	err = tx.QueryRow(ctx, `
		SELECT t.id, t.audience, t.assignee_id FROM activity t
		 WHERE t.source_system = $1 AND t.source_activity_id = $2
		   AND `+openReminderClause("t")+`
		 FOR UPDATE OF t`, EmailRequestTaskSource, source.UUID).Scan(&reminder, &stored, &assignee)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return fmt.Errorf("activities: reading the reminder of %s: %w", source, err)
	}
	want, move := reminderMove(shared, stored)
	if !move {
		return nil
	}
	return writeReminderReachTx(ctx, tx, reminder, stored, want, assignee)
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
		   AND archived_at IS NULL`, reminder, want, stored)
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
// that differs from the reach it carries now.
//
// A reminder already agreeing with its mail is left alone, so a recompute that
// changes nothing writes no audit row. The consumer runs on every audience
// event, and most of them concern a mail whose reminder already agrees.
func reminderMove(mailShared bool, stored string) (string, bool) {
	want := reminderReach(mailShared)
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
// a narrowed reminder, nobody for the workspace's.
//
// An unassigned reminder yields no member rather than a narrowed row nobody is
// named on, which would reach no reader at all.
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
