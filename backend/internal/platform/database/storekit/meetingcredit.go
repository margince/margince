// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HeldCredit is how a figure reads who a held meeting is credited to. It reads
// the host and seats the held transition recorded, together with the live rule.
// Every surface that counts held meetings per colleague builds its filter, its
// list and its seat rows from one value of it. Two surfaces then cannot credit
// one meeting to different people.
type HeldCredit struct {
	// Legacy is true where the meeting has no usable transition and its own host
	// is read. Host is the host the transition recorded. Kept is the uuid[] of
	// seats the held transition credited, or "" for none.
	Legacy, Host, Kept string
}

// LiveCredit reads the meeting as it is today, for a figure with no history.
var LiveCredit = HeldCredit{Legacy: "true", Host: "NULL::uuid"}

// LatestHeldCredit reads the meeting's newest held transition, joined by
// LatestHeldJoinSQL as `held`.
var LatestHeldCredit = HeldCredit{
	Legacy: "held.host_id_at_change IS NULL", Host: "held.host_id_at_change", Kept: "held.credited_seats_at_change",
}

// AsOfHeldCredit reads the transition MeetingHeldAsOfSQL joined as `held`.
func AsOfHeldCredit(legacy string) HeldCredit {
	return HeldCredit{Legacy: legacy, Host: "held.host_id_at_change", Kept: "held.credited_seats_at_change"}
}

// LatestHeldJoinSQL joins the meeting's own newest held transition as `held`. A
// meeting re-held under a new host is credited to that host.
func LatestHeldJoinSQL(meeting string) string {
	return `LEFT JOIN LATERAL (
        SELECT h.host_id_at_change, h.credited_seats_at_change
        FROM activity_meeting_history h
        WHERE h.activity_id = ` + meeting + `.id AND h.status = 'held'
        ORDER BY h.effective_at DESC, h.id DESC
        LIMIT 1
      ) held ON true`
}

// Seats is a row source of the credited seats, as credit(id).
func (c HeldCredit) Seats(meeting string) string {
	return MeetingCreditedSeatsAtChangeSQL(meeting, c.Legacy, c.Host, c.Kept)
}

// CreditsSQL is "seat is credited with this meeting", the form a filter takes.
func (c HeldCredit) CreditsSQL(meeting, seat string) string {
	return "EXISTS (SELECT 1 FROM " + c.Seats(meeting) + " WHERE credit.id = " + seat + ")"
}

// SeatListSQL lists a meeting under every credited seat, and under NULL when it
// credits none. The set-returning call fans a row out only where a statement
// groups by it. A total counts the meeting once. A per-colleague figure counts
// it once for each credited colleague.
func (c HeldCredit) SeatListSQL(meeting string) string {
	return "unnest(COALESCE(NULLIF(ARRAY(SELECT credit.id FROM " + c.Seats(meeting) +
		"), '{}'::uuid[]), ARRAY[NULL::uuid]))"
}

// LowestSeatSQL is the lowest credited seat, NULL when none: one value per
// meeting where a column cannot be a set.
func (c HeldCredit) LowestSeatSQL(meeting string) string {
	return "(SELECT credit.id FROM " + c.Seats(meeting) + " ORDER BY credit.id LIMIT 1)"
}

// HeldCreditSnapshotSQL renders the credited_seats_at_change value a held
// transition inserts for the activity aliased meeting. It is the seats a new
// held transition keeps: the live
// credit and every seat an earlier held transition of the meeting kept. A host
// change, an un-hold or a lost answer then cannot take earlier credit back.
func HeldCreditSnapshotSQL(meeting string) string {
	return `ARRAY(SELECT credit.id FROM ` + MeetingCreditedSeatsSQL(meeting) + `
	    UNION SELECT unnest(prior.credited_seats_at_change) FROM activity_meeting_history prior
	     WHERE prior.activity_id = ` + meeting + `.id AND prior.status = 'held')`
}

// KeepMeetingCreditTx adds the live credit to the newest held transition of a
// meeting. A meeting born held has its guest list written after its first
// transition. The writer of that list calls this in the same transaction, so
// the guests it was held with are the ones kept.
func KeepMeetingCreditTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE activity_meeting_history h SET credited_seats_at_change = `+HeldCreditSnapshotSQL("a")+`
		FROM activity a
		WHERE a.id = $1 AND h.activity_id = a.id AND h.id = (
		      SELECT latest.id FROM activity_meeting_history latest
		       WHERE latest.activity_id = a.id AND latest.status = 'held'
		       ORDER BY latest.effective_at DESC, latest.id DESC LIMIT 1)`, activityID); err != nil {
		return fmt.Errorf("storekit: keeping the credit of the held meeting %s: %w", activityID, err)
	}
	return nil
}
