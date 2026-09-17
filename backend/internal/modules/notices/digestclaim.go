// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// The morning digest: who is owed one, the claim that makes it once a day, and
// the lines a morning is allowed to consider.
//
// TWO PRINCIPALS, and the split is this lane's whole privacy argument. WHO has
// something waiting is answered under the system principal by DigestCandidates,
// which reads no record value — only which KINDS of notice are waiting, routing
// metadata rather than anything a record says. WHAT a digest may quote is the
// recipient's own act, so DigestBody refuses to run as anybody else. Even that
// is not enough on its own: a notice's subject can name a record its recipient
// has since stopped being allowed to open, so the sending lane re-scopes every
// stored reference again at render time.
//
// NOTHING HERE READS THE SEAT ROSTER. Who is a colleague with a morning at all —
// live, human, full seat — is identity's question over a table this module does
// not own, and a module never imports a sibling (ADR-0054 §3). The sending pass
// composes the two: compose/notificationdigestjobs.go asks identity's own
// spelling and narrows what this file answers.
//
// The CLAIM is the morning brief's shape (compose/briefs/briefmail.go): one row
// per (recipient, local day), taken by a conditional insert exactly one tick
// wins, never released, and a failure recording its cause beside it. SMTP
// returns no receipt, so a claim that could be cleared is a retry loop, and a
// retry loop on an hourly pass is how one morning becomes a dozen messages.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// digestWindowSQL is how much of a seat's queue one morning considers: still
// unread, and recorded no earlier than the bound the caller binds at boundAt.
//
// THE PLACEHOLDER IS READ OFF THE ARGUMENT LIST, not written into the text.
// Two statements compose this fragment and they do not carry the same other
// parameters, so a fragment naming a fixed $N is correct only for as long as
// both of them happen to bind the bound in that position — and the day one
// grows a parameter ahead of it, the window silently starts comparing
// created_at against whatever that new argument is.
//
// The self-made stage moves come out through notTheReadersOwnStageMove, which
// every reader of this table composes, so a morning does not tell a rep about
// the stage changes they made themselves.
//
// There is no upper bound. The read itself is the far edge — a notice recorded
// after this statement belongs to the next morning, which is the one whose
// window will reach it.
func digestWindowSQL(boundAt int) string {
	return fmt.Sprintf(`notice.read_at IS NULL
	   AND notice.created_at >= $%d
	   AND `, boundAt) + notTheReadersOwnStageMove
}

// digestReachDays is how far back a morning's window opens, counted in the
// local-day labels LocalDayAt produces.
//
// TWO AND NOT ONE, and the second day is not generosity. A `day` is the
// installation's local date carried at UTC midnight, so in a zone east or west
// of UTC the label sits up to fourteen hours away from the local midnight it
// names — while the message itself goes out at the local morning hour. Reaching
// back one label therefore opens a window that can START AFTER the previous
// morning's message was sent, and a notice recorded in that seam would be
// quoted by neither day. Reaching back two closes the seam in every zone the
// setting can name. The cost is that a notice still unread a day later is
// quoted again the following morning, which is the safe direction for a message
// whose whole job is to say what is still waiting on somebody.
const digestReachDays = 2

// digestRunField names the claim in an audit image, so the word a reader greps
// for is the word the table uses.
const digestRunField = "digest_run"

// digestCandidateQuery answers, per recipient, which kinds of notice are waiting
// on them — the least the enumeration can ask and still place a seat under a
// class.
//
// The anti-join on the claim table is an optimisation of the morning rather than
// the correctness of it: the table's primary key is what makes a second send
// impossible, and a seat who gains a claim between this read and the insert
// simply loses the race there.
//
// `notice` is unaliased so that notTheReadersOwnStageMove — a boolean expression
// over this table's own columns — composes into the WHERE arm under the names it
// was written with.
//
// The statement and its arguments are built together, each placeholder answered
// by the position the value actually took, so the two cannot come apart.
func digestCandidateQuery(day time.Time, only *ids.UUID) (string, []any) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	claimedFor := arg(day)
	window := digestWindowSQL(arg(digestReach(day)))
	seat := arg(only)
	return fmt.Sprintf(`
	SELECT notice.recipient_user_id, notice.kind
	  FROM notice
	 WHERE %[1]s
	   AND ($%[2]d::uuid IS NULL OR notice.recipient_user_id = $%[2]d)
	   AND NOT EXISTS (
	       SELECT 1 FROM notification_digest_run r
	        WHERE r.user_id = notice.recipient_user_id AND r.digest_date = $%[3]d)
	 GROUP BY notice.recipient_user_id, notice.kind
	 ORDER BY notice.recipient_user_id, notice.kind`, window, seat, claimedFor), args
}

// waitingSeat is one recipient and the kinds of notice waiting on them.
type waitingSeat struct {
	user  ids.UserID
	kinds []string
}

// DigestCandidates lists the recipients holding something this morning could
// carry: an unread notice in the window, of a class they asked for in a batch,
// with no claim yet for the day.
//
// It runs under the SYSTEM principal and reads no record value — only the claim
// table, the routing choices, and the kinds waiting. It says nothing about
// whether a recipient is a colleague with a morning at all; the sending pass
// narrows this list against identity's own roster, because that is a question
// about a table this module does not own. What a digest may QUOTE is a third
// question, asked under the recipient's own principal, in DigestBody.
func (s *Store) DigestCandidates(ctx context.Context, day time.Time) ([]ids.UserID, error) {
	var due []ids.UserID
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		waiting, txErr := waitingSeats(ctx, tx, day, nil)
		if txErr != nil {
			return txErr
		}
		for _, seat := range waiting {
			batched, txErr := newBatchedClasses(seat.user).anyOf(ctx, tx, seat.kinds)
			if txErr != nil {
				return txErr
			}
			if batched {
				due = append(due, seat.user)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("notices: listing the seats due a digest: %w", err)
	}
	return due, nil
}

// DigestStillDue re-asks the candidate question of ONE recipient, in the instant
// before their claim is spent.
//
// The candidate list is read once for a whole workspace and the claim is spent
// per seat, so this covers the window between the two: a colleague whose notices
// were all answered on screen since, or who moved the class back off the batch,
// drops out here. The ORDER is the point. Asking after the claim would burn this
// colleague's one attempt for the day on a class they had just moved back to
// their screen, so the morning they changed their mind on could never be sent
// and nothing would say why.
func (s *Store) DigestStillDue(ctx context.Context, user ids.UserID, day time.Time) (bool, error) {
	var due bool
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		waiting, txErr := waitingSeats(ctx, tx, day, &user)
		if txErr != nil || len(waiting) == 0 {
			return txErr
		}
		due, txErr = newBatchedClasses(user).anyOf(ctx, tx, waiting[0].kinds)
		return txErr
	}); err != nil {
		return false, fmt.Errorf("notices: re-reading whether the morning is still owed: %w", err)
	}
	return due, nil
}

// ClaimDigestRun takes this seat's ONE digest for the day, or reports that
// somebody already has it.
//
// A conditional insert exactly one tick can win. Everything the caller does
// after it is allowed to fail and lose the message; nothing after it is allowed
// to produce a second one, and nothing ever releases it.
func (s *Store) ClaimDigestRun(ctx context.Context, user ids.UserID, day time.Time) (bool, error) {
	var claimed bool
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, txErr := tx.Exec(ctx, `
			INSERT INTO notification_digest_run (user_id, digest_date)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, user, day)
		if txErr != nil {
			return fmt.Errorf("claiming the day: %w", txErr)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		claimed = true
		// The entity is the SEAT, the way their delivery settings are audited:
		// the claim row carries no id of its own, and what happened is a fact
		// about one colleague's morning.
		_, txErr = storekit.Audit(ctx, tx, "update", "user", user.UUID,
			map[string]any{digestRunField: nil},
			map[string]any{digestRunField: day.Format(time.DateOnly)})
		return txErr
	}); err != nil {
		return false, fmt.Errorf("notices: claiming the seat's morning digest: %w", err)
	}
	return claimed, nil
}

// DigestFailed records why the claimed morning produced no message.
//
// It does NOT release the claim, and that is the point: the attempt is spent
// either way, and a caller that could clear it would have rebuilt the retry
// loop this design refuses. A SKIP is not a failure and never reaches here —
// writing "the seat changed their mind" into the column an operator reads to
// find out why a message did not arrive would bury the relay failures it exists
// for.
func (s *Store) DigestFailed(ctx context.Context, user ids.UserID, day time.Time, cause string) error {
	cause = truncate(cause, maxMailErrorRunes)
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, txErr := tx.Exec(ctx, `
			UPDATE notification_digest_run SET mail_error = $3
			 WHERE user_id = $1 AND digest_date = $2`, user, day, cause)
		if txErr != nil {
			return fmt.Errorf("recording the cause: %w", txErr)
		}
		if tag.RowsAffected() == 0 {
			// A cause beside no claim describes a send that never happened.
			return apperrors.ErrNotFound
		}
		_, txErr = storekit.Audit(ctx, tx, "update", "user", user.UUID,
			map[string]any{digestRunField + "_error": nil},
			map[string]any{digestRunField + "_error": cause})
		return txErr
	}); err != nil {
		return fmt.Errorf("notices: recording why the morning digest was not sent: %w", err)
	}
	return nil
}

// DigestBody answers the recipient's own unread batched notices for the
// morning, newest first.
//
// THE RECIPIENT'S OWN ACT. The caller binds this seat's authority first, and the
// parameter has to name that same seat: a digest read under anybody else's
// principal would resolve every record reference in it against somebody else's
// scope, which is the one mistake this lane exists to make impossible.
//
// It is the CONTENT read and it stops at the text. Whether a notice may name the
// record it points at is re-asked by the sending lane, against the recipient's
// scope, at the moment the message is rendered — the stored reference is a fact
// about the day the notice was written, never a permission.
func (s *Store) DigestBody(ctx context.Context, user ids.UserID, day time.Time) ([]Notice, error) {
	seat, err := actingSeat(ctx, "reading your morning digest")
	if err != nil {
		return nil, err
	}
	if seat != user.UUID {
		return nil, fmt.Errorf(
			"notices: a morning digest is read under its own recipient's authority: %w",
			apperrors.ErrPermissionDenied)
	}
	var batched []Notice
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		held, txErr := unreadInWindow(ctx, tx, user, day)
		if txErr != nil {
			return txErr
		}
		batched, txErr = onlyBatched(ctx, tx, user, held)
		return txErr
	}); err != nil {
		return nil, fmt.Errorf("notices: reading the morning digest: %w", err)
	}
	return batched, nil
}

// waitingSeats runs the candidate query, for every recipient or for one.
//
// One statement for both, because they are one question asked at two moments —
// the sweep and the re-ask before the claim — and a second spelling is how the
// two would come to disagree about who is owed a morning.
func waitingSeats(ctx context.Context, tx pgx.Tx, day time.Time, only *ids.UserID) ([]waitingSeat, error) {
	var seat *ids.UUID
	if only != nil {
		seat = &only.UUID
	}
	query, args := digestCandidateQuery(day, seat)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing the recipients with notices waiting: %w", err)
	}
	defer rows.Close()
	var waiting []waitingSeat
	for rows.Next() {
		var user ids.UserID
		var kind string
		if scanErr := rows.Scan(&user, &kind); scanErr != nil {
			return nil, scanErr
		}
		if n := len(waiting); n > 0 && waiting[n-1].user == user {
			waiting[n-1].kinds = append(waiting[n-1].kinds, kind)
			continue
		}
		waiting = append(waiting, waitingSeat{user: user, kinds: []string{kind}})
	}
	return waiting, rows.Err()
}

// unreadInWindow reads the recipient's own unread notices for the morning,
// whole rows, newest first.
//
// UNBOUNDED on purpose. The window and the single recipient are what bound it,
// and a LIMIT here would make the message's own tail count a lie: it quotes a
// handful and says how many more are waiting, and "how many more" has to be all
// of them.
func unreadInWindow(ctx context.Context, tx pgx.Tx, user ids.UserID, day time.Time) ([]Notice, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	recipient := arg(user)
	window := digestWindowSQL(arg(digestReach(day)))
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT notice.id, notice.kind, notice.subject, notice.body,
		       notice.target_type, notice.target_id, notice.created_at, notice.origin
		  FROM notice
		 WHERE notice.recipient_user_id = $%d
		   AND %s
		 ORDER BY notice.created_at DESC, notice.id DESC`, recipient, window), args...)
	if err != nil {
		return nil, fmt.Errorf("listing the morning's unread notices: %w", err)
	}
	defer rows.Close()
	var held []Notice
	for rows.Next() {
		var n Notice
		// Both halves are nullable and the table pairs them, so either arriving
		// alone is a row the constraint should have refused.
		var targetType *string
		var targetID *ids.UUID
		if scanErr := rows.Scan(&n.ID, &n.Kind, &n.Subject, &n.Body,
			&targetType, &targetID, &n.CreatedAt, &n.Origin); scanErr != nil {
			return nil, scanErr
		}
		if targetType != nil && targetID != nil {
			n.Target = Target{Type: *targetType, ID: *targetID}
		}
		held = append(held, n)
	}
	return held, rows.Err()
}

// onlyBatched keeps the notices whose class this seat asked to receive in a
// batch, and drops the rest.
//
// Read AFTER the rows above are drained and closed: each preference lookup is
// its own query on this transaction, and pgx refuses a second query while a
// cursor is open.
func onlyBatched(ctx context.Context, tx pgx.Tx, user ids.UserID, held []Notice) ([]Notice, error) {
	classes := newBatchedClasses(user)
	batched := make([]Notice, 0, len(held))
	for _, n := range held {
		wanted, err := classes.wants(ctx, tx, n.Kind)
		if err != nil {
			return nil, err
		}
		if wanted {
			batched = append(batched, n)
		}
	}
	return batched, nil
}

// batchedClasses answers, once per class, whether a seat asked for that class
// in a batch.
//
// Through ClassFor and DeliveryFor rather than a `delivery = 'digest'` join: the
// kind-to-class placement is this package's, and so is what a never-chosen class
// falls back to, so a predicate in SQL would be a second and quieter copy of
// both — and the quiet copy is the one that goes on answering after the loud one
// moves.
//
// Memoised because a morning's notices cluster into two or three classes, and a
// lookup per notice would ask the same question of the same row a dozen times.
type batchedClasses struct {
	user    ids.UserID
	decided map[string]bool
}

func newBatchedClasses(user ids.UserID) *batchedClasses {
	return &batchedClasses{user: user, decided: map[string]bool{}}
}

func (b *batchedClasses) wants(ctx context.Context, tx pgx.Tx, kind string) (bool, error) {
	class, err := ClassFor(kind)
	if err != nil {
		return false, err
	}
	if wanted, decided := b.decided[class]; decided {
		return wanted, nil
	}
	delivery, err := DeliveryFor(ctx, tx, b.user, class)
	if err != nil {
		return false, err
	}
	wanted := delivery == DeliveryDigest
	b.decided[class] = wanted
	return wanted, nil
}

// anyOf reports whether any of these kinds reaches this seat in a batch.
func (b *batchedClasses) anyOf(ctx context.Context, tx pgx.Tx, kinds []string) (bool, error) {
	for _, kind := range kinds {
		wanted, err := b.wants(ctx, tx, kind)
		if err != nil {
			return false, err
		}
		if wanted {
			return true, nil
		}
	}
	return false, nil
}

// digestReach is where this morning's window opens — see digestReachDays for
// why it is two labels back and not one.
func digestReach(day time.Time) time.Time { return day.AddDate(0, 0, -digestReachDays) }
