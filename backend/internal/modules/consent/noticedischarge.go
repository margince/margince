// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Discharging a disclosure duty by actually sending one.
//
// A notice case records that the installation owes a contact an Art. 13 or
// Art. 14 disclosure and by when. Recording it is half the control; the other
// half is that sending the disclosure MOVES the case, so the queue empties as
// duties are met rather than growing forever beside a mail nobody connected to
// it. Without this the record-confirmation route was a string in allowed_routes
// that no writer ever honoured.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noticeResendCooldown is how long a case rests after a disclosure was sent for
// it before another send may move it again.
//
// The duty is discharged by ONE message, so a second one inside the window is
// not a second discharge — it is the same rep clicking twice, or a screen
// retrying. Counting that as progress would let a case be "queued" repeatedly
// while its subject received one mail, and the attempts column would read as
// effort nobody made. Thirty days is the plan's number and long enough that a
// genuine re-send after a bounce still falls outside it.
//
// The window is EXACT for any case discharged since last_sent_at existed, and a
// floor for the rest. That column records when a disclosure was actually sent,
// which is the question the cooldown is asking; updated_at moves for any write,
// so a merge relinking the contact (contacts/mergerelink.go) used to extend
// somebody's cooldown by up to a month. The coalesce keeps the old behaviour
// for rows stamped before the column existed, which errs toward not re-sending
// — the safe direction, since the duty stays visible on the queue either way
// and a second mail to somebody who already got one is the harm this bounds.
//
// The attempts = 0 arm is what keeps the floor from swallowing the FIRST send:
// a freshly opened case has had no disclosure at all, and one whose updated_at
// happens to be recent would otherwise sit out its own first mail, which is the
// exact failure this whole change exists to end. Giving the row its own
// last_sent_at would make the window exact; that is a migration, and this does
// not need one to be correct.
const noticeResendCooldown = 30 * 24 * time.Hour

// dischargeNoticeCases moves this contact's live cases that the sent disclosure
// discharges onto `queued`, and answers how many it moved.
//
// QUEUED RATHER THAN COMPLETED, and the distinction is the honest one: this
// runs when the mail is staged, and a staged mail is not a received one. The
// case leaves the "nobody has done anything" reading without claiming the
// subject was actually told — the delivery row is what knows that, and it
// changes as the dispatcher learns more.
//
// The route is matched against allowed_routes rather than assumed, so a case
// the product cannot discharge this way (an Art. 13 form case, whose route is
// the reply somebody else sends) is left where it is.
//
// IT DISCHARGES WHAT EXISTS NOW, and a case opened later is not reached. The
// contact.created consumer opens cases asynchronously, so a confirm mail sent in
// the window between a contact committing and that consumer running moves
// nothing, and the case then opens as owed. That failure is the SAFE one: the
// duty stays on the queue, visibly undischarged, and the worst outcome is a
// second disclosure to somebody who already had one. Reconciling it would mean
// reading the contact's sent mail from here, which is a delivery question this
// module cannot answer, and the alternative — assuming a recent mail discharged
// a duty recorded after it — is the unsafe direction.
//
// Runs on the CALLER'S transaction, so the case transition, the token, the mail
// and the audit rows commit together. A discharge recorded against a mail that
// rolled back would be a duty marked handled by a message that never existed —
// and the caller gates the call on the mail having actually been STAGED, which
// the transaction alone does not give: an installation with no lane mints a link,
// stages nothing, and commits.
//
// THIS IS THE FIRST WRITER OF `attempts`, so it fixes what the column means:
// how many disclosures have been SENT for this duty, not how many times
// anything touched the row. A case already queued that is sent to again past
// the cooldown counts a second attempt and stays queued — the duty was not
// discharged by the first message, and the count is what says so.
func dischargeNoticeCases(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, route string,
	now time.Time, deliveryID ids.UUID,
) (int, error) {
	// The cooldown boundary is computed HERE, off the injected clock, rather
	// than as SQL arithmetic on now(): the caller's `now` is what a test drives
	// a deadline with, and a statement reading the database's own clock would
	// answer a different question from the one the test asked.
	restedSince := now.Add(-noticeResendCooldown)
	// The prior state comes from a self-join rather than RETURNING, which in
	// Postgres sees only the new row. The write shape refuses an update audited
	// with no before-image, and rightly: "it is queued now" without "it was open
	// then" cannot tell a discharge from a re-statement of one.
	rows, err := tx.Query(ctx, `
		UPDATE privacy_notice_case c
		   SET state = $4, attempts = c.attempts + 1, updated_at = $3,
		       delivery_id = $7, last_sent_at = $3
		  FROM privacy_notice_case prior
		 WHERE prior.id = c.id
		   AND c.contact_id = $1
		   AND c.state = ANY($5)
		   AND $2 = ANY(c.allowed_routes)
		   AND (c.attempts = 0 OR coalesce(c.last_sent_at, c.updated_at) <= $6)
		RETURNING c.id, c.rule, c.attempts, prior.state, prior.attempts`,
		contactID, route, now, string(NoticeQueued),
		dischargeableNoticeStates(), restedSince, deliveryID)
	if err != nil {
		return 0, fmt.Errorf("record that a disclosure was sent for this contact's open duties: %w", err)
	}
	type moved struct {
		id          ids.UUID
		rule        string
		attempts    int
		wasState    string
		wasAttempts int
	}
	// Drained and closed BEFORE the audit writes below, which is why these are
	// hand-closed rather than deferred: the audit runs further queries on this
	// same transaction, and a connection cannot carry an open row set and a new
	// query at once.
	var discharged []moved
	for rows.Next() {
		var m moved
		if err := rows.Scan(&m.id, &m.rule, &m.attempts, &m.wasState, &m.wasAttempts); err != nil {
			rows.Close()
			return 0, fmt.Errorf("read a discharged notice case: %w", err)
		}
		discharged = append(discharged, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("record that a disclosure was sent for this contact's open duties: %w", err)
	}
	for _, m := range discharged {
		// Audited per case, not once for the batch: each is its own compliance
		// record, and "when was this duty discharged and by what" is asked of
		// one case at a time.
		before := map[string]any{fieldState: m.wasState, "attempts": m.wasAttempts}
		if _, err := storekit.Audit(ctx, tx, "update", "privacy_notice_case", m.id, before, map[string]any{
			fieldRule: m.rule, fieldState: string(NoticeQueued),
			"route": route, "attempts": m.attempts,
		}); err != nil {
			return 0, fmt.Errorf("audit the discharged notice case: %w", err)
		}
	}
	return len(discharged), nil
}

// dischargeableNoticeStates are the states a fresh disclosure can move a case
// out of: still owed, with nothing stopping it being sent.
//
// DERIVED from "still owed" rather than re-listed, so a sixth state reaches this
// by existing — the queue's own read is derived the same way, and two lists of
// one vocabulary drift until they disagree about whether a duty is open.
//
// Blocked is the one subtraction. A blocked case names an obstacle, and sending
// past it would leave the row asserting both that a disclosure went out and that
// one could not — which the table's blocked_shape CHECK also refuses, so without
// this filter one blocked case would make every confirm mail to that contact fail
// on a raw constraint violation.
func dischargeableNoticeStates() []string {
	return slices.DeleteFunc(unresolvedNoticeStates(), func(state string) bool {
		return state == string(NoticeBlocked)
	})
}

// noticeRouteFor answers which disclosure route a mailed link discharges, and
// whether it discharges one at all.
//
// The two vocabularies happen to spell the record-confirmation case the same
// way, and this function exists so that coincidence is never what makes the
// discharge work. A link kind is what a mail ASKS; a route is how a duty may be
// MET; they are separate closed sets owned by different rules, and either could
// be renamed without the other.
//
// A consent link discharges nothing: it asks whether the subject wants
// marketing, which is a different question from telling them we hold their
// data. Answering it leaves the Art. 14 duty exactly as owed as before.
func noticeRouteFor(linkKind string) (string, bool) {
	if linkKind == LinkRecordConfirmation {
		return noticeRouteRecordConfirmation, true
	}
	return "", false
}
