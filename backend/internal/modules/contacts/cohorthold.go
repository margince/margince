// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The lock order of a cohort repair: every activity the pass may write, in id
// order, and only then the contact.
//
// The capture ensure and the meeting roster settle lock an activity before the
// contact they file it under. A repair that locked the contact first and then
// linked the activity could close a cycle with either one.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// busyActivities is what the hold does with an activity another transaction
// has locked.
type busyActivities string

const (
	// waitForBusyActivities waits for it. Safe only while this transaction
	// holds no lock on the contact.
	waitForBusyActivities busyActivities = ""
	// skipBusyActivities leaves it for the next pass. Its holder may be waiting
	// for the contact this transaction already holds.
	skipBusyActivities busyActivities = " SKIP LOCKED"
)

// holdCohortActivities bounds every write to the activity rows it locks.
// Its mode matches later link and audience writes, avoiding an upgrade while
// staying compatible with an import's key-share lock.
func holdCohortActivities(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, busy busyActivities,
) ([]ids.UUID, error) {
	var canonical ids.ContactID
	err := tx.QueryRow(ctx,
		`SELECT coalesce(merged_into_id, id) FROM contact WHERE id = $1`, contactID).Scan(&canonical)
	if errors.Is(err, pgx.ErrNoRows) {
		// The locked read that follows reports the missing contact.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("contacts: resolving the contact whose cohort to hold: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id FROM activity a
		 WHERE a.id = ANY(ARRAY(
		       (SELECT a.id FROM activity a
		         WHERE `+capturedCohortMailSQL+`
		         ORDER BY a.occurred_at DESC, a.id
		         LIMIT @batch)
		       UNION ALL
		       (SELECT a.id FROM activity a
		         WHERE a.id IN (`+cohortAttendanceSQL+`)
		           AND `+attendedMeetingSQL+`
		         ORDER BY a.occurred_at DESC, a.id
		         LIMIT @batch)
		       UNION ALL
		       (SELECT sub.activity_id FROM activity_participant sub
		         WHERE lower(sub.address) = ANY(ARRAY(`+cohortAddressesSQL+`))
		           AND `+promotableAddressParticipantSQL("sub", "@cohort_contact")+`
		         ORDER BY sub.id
		         LIMIT @batch)
		       UNION ALL
		       (SELECT sub.activity_id FROM contact_channel_identity owner
		         JOIN activity_participant sub ON sub.channel_user_id = owner.channel_user_id
		         WHERE owner.contact_id = @cohort_contact AND owner.archived_at IS NULL
		           AND sub.channel_user_id IS NOT NULL
		           AND `+promotableChannelParticipantSQL("sub", "@cohort_contact")+`
		         ORDER BY sub.id
		         LIMIT @batch)))
		 ORDER BY a.id
		 FOR NO KEY UPDATE`+string(busy), cohortArgs(canonical, nil))
	if err != nil {
		return nil, fmt.Errorf("contacts: locking a contact's cohort: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("contacts: locking a contact's cohort: %w", err)
	}
	return held, nil
}

// The hold runs on every contact event, so its arms start from the contact's own
// rows and seek activity_participant by index. Each arm then applies the write's
// full predicate, so the seek only narrows which rows are tested.
const (
	// cohortAttendanceSQL lists the activities with a participant row naming the
	// contact or a record merged into it, as attendedMeetingSQL resolves them.
	cohortAttendanceSQL = `SELECT ap.activity_id FROM activity_participant ap
		 WHERE ap.contact_id IN (SELECT att.id FROM contact att
		        WHERE att.id = @cohort_contact OR att.merged_into_id = @cohort_contact)`
	// cohortAddressesSQL is the contact's live addresses in the form
	// idx_aparticipant_address indexes.
	cohortAddressesSQL = `SELECT lower(pe.email) FROM contact_email pe
		 WHERE pe.contact_id = @cohort_contact AND pe.archived_at IS NULL`
)

// cohortArgs binds the names the cohort statements share.
func cohortArgs(contactID ids.ContactID, held []ids.UUID) pgx.NamedArgs {
	return pgx.NamedArgs{
		"cohort_contact": contactID, "held": held,
		"batch": cohortRepairBatch, "ceiling": maxMeetingLinksPerActivity,
	}
}
