// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which conversations the extractor asks about, and where it got to.
//
// The queue is derived, not maintained: a thread is due when something has
// arrived on it since the watermark says it was last read. There is no work
// table to drift, and a thread nobody has written on costs nothing forever.
//
// A thread is asked about only once it has SETTLED. Reading a conversation
// mid-exchange produces events that the next message contradicts — "they are
// ending the contract" written while the sentence it came from was still being
// negotiated. Six hours is the pin (SIG-PARAM-7); it is the same posture the
// capture passes take toward mail that is still arriving.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	// extractSettleHours is how long a thread must be quiet before it is read.
	extractSettleHours = 6
	// extractThreadMessages is how far back one call looks. A material event
	// is stated in the exchange it belongs to, not six months earlier, and a
	// longer window buys older text at the price of the recent text's weight.
	extractThreadMessages = 6
	// extractBodyLimit truncates each body for the prompt, as capture-classify
	// truncates its own (AIRT-PARAM-35).
	extractBodyLimit = 1500
	// extractThreadCap bounds one pass, so a workspace that has just connected
	// a mailbox does not spend its whole model budget on the first tick.
	extractThreadCap = 200
	// extractRefusalCap is how many times ONE conversation state may be refused
	// before it is parked. It exists because the cap above is a scarce resource:
	// a thread that stays due forever holds a slot in every pass, and enough of
	// them read nothing but themselves while the backlog behind them starves.
	// Three, because the model lane already retries and escalates internally —
	// this counts whole readings, not attempts, so three is three escalated
	// disagreements about the same text.
	extractRefusalCap = 3
	// extractParkFor is how long a conversation stays parked before it is
	// offered again.
	//
	// Parking has to expire. What refuses a reading is the pair of a text and a
	// model, and only one of those is fixed: a routing change, a new model
	// version, a corrected prompt or a raised cap can all make a conversation
	// readable that was not readable last week. Parking it until new mail
	// happens to arrive would drop, permanently and silently, the material
	// events in every thread that never receives another message — which is
	// most finished conversations, and a finished conversation is exactly where
	// "the contract ended" is stated.
	//
	// A week: long enough that a poisoned thread costs one reading a week
	// instead of one an hour, short enough that a fix reaches the backlog
	// without anybody replaying anything by hand.
	extractParkFor = 7 * 24 * time.Hour
)

// threadMessage is one message of a conversation as the prompt sees it.
type threadMessage struct {
	ID        ids.UUID
	Direction string
	Subject   string
	Body      string
	At        time.Time
	// UnreadRunes is how much of this body extractBodyLimit left behind.
	//
	// Carried rather than inferred, because it cannot be inferred: a body cut
	// at the limit and a body that was exactly that long reach the prompt
	// identically, and the reading is drawn from the head either way. The
	// remainder is the half that says which happened, and how far the model's
	// view of the exchange falls short of it.
	UnreadRunes int
}

// settledThread is one conversation due for a read, with the account it
// belongs to already resolved — an event about nobody is not an event this
// product can file.
type settledThread struct {
	Key       string
	CompanyID ids.UUID
	// Newest is the instant the watermark advances to, read at the same time
	// as the messages so a message arriving mid-pass is not skipped: it is
	// newer than what this pass records, so the next pass picks the thread up.
	Newest time.Time
	// Count is how many messages the conversation held when this pass read it.
	// The timestamp alone cannot see a message inserted at the same instant, or
	// a backfill filling in older ones; the count changes for both.
	Count int
	// ReadTo and ReadFrom are the two ends a PREVIOUS read reached: the newest
	// message it covered and the oldest. Both nil on a thread never scanned.
	//
	// Two ends and not one, because a conversation grows at both. Newer than
	// ReadTo is new mail; older than ReadFrom is a backfill; and a scan that
	// tracked only how far forward it had got could not see the second at all.
	ReadTo   *time.Time
	ReadFrom *time.Time
	// ReadFromID pairs with ReadFrom, because an INSTANT IS NOT A MESSAGE
	// BOUNDARY: mail imported in bulk shares an occurred_at routinely, and a
	// cursor that is a timestamp alone either skips the rest of a group or
	// re-reads it forever. (occurred_at, id) is the order the window walks in.
	ReadFromID *ids.UUID
	// ReadFromNow and ReadFromIDNow are the oldest message THIS pass read,
	// which becomes the cursor for the next one. Set by threadMessages, because
	// the window it chose is the only thing that knows.
	ReadFromNow   *time.Time
	ReadFromIDNow *ids.UUID
	// PrivateTo is the one reader this conversation answers to, or the zero
	// value when every message on it is workspace-readable.
	//
	// What the model writes about a conversation is only as shareable as the
	// conversation. Capture auto-creates contacts owner-private, and a message
	// filed against nobody else is readable by its capturing user alone — so a
	// summary of it, filed on an account the whole workspace can see, would
	// disclose exactly what the private contact protects.
	PrivateTo ids.UUID
	Messages  []threadMessage
}

// dueThreads lists the conversations that have settled and have moved since
// they were last read, newest first.
//
// A conversation's account comes from the three-arm walk (the message's own
// link, its deal's account, the employer of the contact it is about) rather
// than a direct company link. Capture files mail against the CONTACT it was
// with, so a direct match resolves nothing on real correspondence — an account
// is reached through its contacts, or not at all.
//
// The walk is joined rather than applied as a predicate because the question
// here is which account a thread belongs to, not whether it belongs to a known
// one — and it is a LEFT join because the two things the aggregate computes are
// different questions over different rows:
//
//   - WHEN the conversation last moved, and how many messages it holds, is
//     asked of EVERY message on the thread. Counting only the resolvable ones
//     would let a thread look settled while a message nobody can place arrived
//     minutes ago, and the settle window exists precisely to keep the model out
//     of a conversation still in progress.
//   - WHICH account it belongs to is asked only of the messages that reach one;
//     count(DISTINCT) ignores the NULLs a LEFT join leaves behind.
//
// The company resolution is deliberately strict: exactly one company across
// the whole thread. A conversation touching two accounts would have its events
// filed against whichever the join happened to pick, and a signal on the wrong
// account is worse than no signal — it is a claim the reader cannot trace back.
// A contact with two live employers makes their threads ambiguous by the same
// rule and skips them; filtering the walk down to a primary employer would buy
// those threads back by guessing, which is the thing being refused.
//
// A message whose contact reaches no account at all still reaches the PROMPT —
// threadMessages reads the conversation by thread_key alone. Resolution decides
// whose conversation this is, not which of its messages the model may read.
func dueThreads(ctx context.Context, tx pgx.Tx, now time.Time, limit int) ([]settledThread, error) {
	settled := now.Add(-extractSettleHours * time.Hour)
	rows, err := tx.Query(ctx, dueThreadsQuery,
		settled, limit, extractRefusalCap, now.Add(-extractParkFor))
	if err != nil {
		return nil, fmt.Errorf("list the threads due for a read: %w", err)
	}
	defer rows.Close()
	var due []settledThread
	for rows.Next() {
		var thread settledThread
		var privateTo *ids.UUID
		if err := rows.Scan(&thread.Key, &thread.CompanyID,
			&thread.Newest, &thread.Count, &privateTo,
			&thread.ReadTo, &thread.ReadFrom, &thread.ReadFromID); err != nil {
			return nil, err
		}
		if privateTo != nil {
			thread.PrivateTo = *privateTo
		}
		due = append(due, thread)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range due {
		messages, err := threadMessages(ctx, tx, &due[i])
		if err != nil {
			return nil, err
		}
		due[i].Messages = messages
	}
	return due, nil
}

// recordThreadRefusal counts one refused reading of this exact conversation
// state, and does NOT advance the watermark.
//
// The two are different facts and the row keeps them apart. last_activity_at
// and message_count say what has been READ; refusals says how often the model
// failed to read it. Advancing the watermark here would retire the thread and
// lose whatever it says; counting without pinning would let a growing
// conversation inherit the refusals of text it no longer contains.
func recordThreadRefusal(
	ctx context.Context, tx pgx.Tx, thread settledThread, now time.Time,
) (int, error) {
	var refusals int
	if err := tx.QueryRow(ctx, `
		INSERT INTO signal_thread_scan
		  (thread_key, last_activity_at, message_count, scanned_at,
		   refusals, refused_activity_at, refused_message_count)
		VALUES ($1, '-infinity', 0, $4, 1, $2, $3)
		ON CONFLICT (thread_key) DO UPDATE
		   SET refusals = CASE
		         WHEN signal_thread_scan.refused_activity_at IS NOT DISTINCT FROM excluded.refused_activity_at
		          AND signal_thread_scan.refused_message_count IS NOT DISTINCT FROM excluded.refused_message_count
		         THEN signal_thread_scan.refusals + 1
		         -- A different state: the count starts again, because these are
		         -- refusals of text the model has not been shown before.
		         ELSE 1 END,
		       refused_activity_at = excluded.refused_activity_at,
		       refused_message_count = excluded.refused_message_count,
		       -- scanned_at is when this conversation was last LOOKED at, which
		       -- is what the park window is measured from.
		       scanned_at = excluded.scanned_at
		RETURNING refusals`,
		thread.Key, thread.Newest, thread.Count, now).Scan(&refusals); err != nil {
		return 0, fmt.Errorf("count the refused reading: %w", err)
	}
	return refusals, nil
}

// markThreadScanned records what THIS pass read — the newest instant and the
// message count it saw — never now() and never a fresh count. A thread that
// grew while the model was answering is left looking unread, so the next pass
// reads it again: a repeat read writes nothing new (the fingerprint holds),
// while a skipped one loses the event for good.
//
// last_activity_at takes greatest() because it must never go backwards; the
// count is overwritten, because a backfill that adds older messages legitimately
// lowers nothing and raises the count, and clamping it would hide exactly the
// change it exists to notice.
func markThreadScanned(ctx context.Context, tx pgx.Tx, thread settledThread, now time.Time) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO signal_thread_scan
		  (thread_key, last_activity_at, message_count, scanned_at,
		   resolved_company_id, scanned_from, scanned_from_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (thread_key) DO UPDATE
		   SET last_activity_at = greatest(signal_thread_scan.last_activity_at, excluded.last_activity_at),
		       -- The oldest message ever read, so it only ever moves EARLIER.
		       -- least() over a null is null in Postgres only for the operands
		       -- it has none of — least(x, NULL) is x — which is what carries a
		       -- cursor through a pass that read the newest window and reached
		       -- nothing older.
		       -- The pair moves together, and only EARLIER. A window that read
		       -- nothing older than the cursor leaves it where it was, which is
		       -- what carries it through a pass that read the newest end.
		       scanned_from_id = CASE
		         WHEN excluded.scanned_from IS NULL THEN signal_thread_scan.scanned_from_id
		         WHEN signal_thread_scan.scanned_from IS NULL THEN excluded.scanned_from_id
		         WHEN (excluded.scanned_from, excluded.scanned_from_id)
		            < (signal_thread_scan.scanned_from, signal_thread_scan.scanned_from_id)
		           THEN excluded.scanned_from_id
		         ELSE signal_thread_scan.scanned_from_id END,
		       scanned_from = CASE
		         WHEN excluded.scanned_from IS NULL THEN signal_thread_scan.scanned_from
		         WHEN signal_thread_scan.scanned_from IS NULL THEN excluded.scanned_from
		         WHEN (excluded.scanned_from, excluded.scanned_from_id)
		            < (signal_thread_scan.scanned_from, signal_thread_scan.scanned_from_id)
		           THEN excluded.scanned_from
		         ELSE signal_thread_scan.scanned_from END,
		       message_count = excluded.message_count,
		       scanned_at = excluded.scanned_at,
		       -- WHICH account this reading was for. Overwritten, never
		       -- greatest()-style clamped: the account is not a high-water mark,
		       -- it is what the walk resolves to now.
		       resolved_company_id = excluded.resolved_company_id,
		       -- A reading landed, so the earlier refusals were about a model
		       -- that could not do it then, not a conversation that cannot be
		       -- done. Left standing they would park the thread on its next
		       -- refusal however long ago the others were.
		       refusals = 0,
		       refused_activity_at = NULL,
		       refused_message_count = NULL`,
		thread.Key, thread.Newest, thread.Count, now, thread.CompanyID,
		thread.ReadFromNow, thread.ReadFromIDNow); err != nil {
		return fmt.Errorf("record where the read got to: %w", err)
	}
	return nil
}
