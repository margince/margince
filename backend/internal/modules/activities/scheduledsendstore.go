// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Reading and changing a message that is waiting to be sent: the rep's own
// list, one message's detail, moving its moment, and withdrawing it. The
// scheduling decision itself lives beside this; firing lives in
// scheduledsendfire.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// scheduledSendColumns is the read shape, spelled once so the list and the
// detail read cannot drift into scanning different rows.
// scheduledSendStatusExpr is the status a REP sees, spelled once because the
// projection and the filter must agree: rendering a derived state while
// filtering the raw column makes `?status=sent` return nothing and
// `?status=released` return rows that read "sent".
const scheduledSendStatusExpr = `
	CASE WHEN status = 'released'
	       AND EXISTS (SELECT 1 FROM comms_outbound o
	                    WHERE o.id = scheduled_send.delivery_id AND o.status = 'sent')
	     THEN 'sent' ELSE status END`

const scheduledSendColumns = `
	id,
	-- 'released' is where the fire transaction leaves the row: the message has
	-- been handed to the delivery machinery and the provider has not been called
	-- yet. When the provider confirms receipt the DELIVERY records it, and the
	-- scheduled send follows — a message this system sent reads "sent" whether a
	-- rep scheduled it or pressed the button (ADR-0104 §5, DRAFT-AC-N-10a).
	--
	-- DERIVED at read rather than written by a second writer. comms owns the
	-- receipt and this table belongs to activities, so a cross-module write
	-- would be a second place for the two to disagree about one message. Reading
	-- the delivery's own status cannot drift from it.
	` + scheduledSendStatusExpr + ` AS status,
	` + scheduleInstantSQL + ` AS scheduled_at, scheduled_tz, scheduled_wall, origin_kind,
	anchor_activity_id, payload, scheduled_by, activity_id,
	held_reason, version, created_at, updated_at, origin_links, also_links, payload_version, principal_kind`

// ListScheduledSends returns the caller's pending and held messages.
func (s *Store) ListScheduledSends(ctx context.Context, status string) ([]ScheduledSend, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	// The surrounding transaction only requires a workspace; this predicate is
	// what narrows the read to the author.
	own, err := ScheduledSendReadableSQL(ctx, "scheduled_send", arg)
	if err != nil {
		return nil, err
	}
	statusAt := arg(status)
	var out []ScheduledSend
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT`+scheduledSendColumns+`
			  FROM scheduled_send
			 WHERE %s AND ($%[2]d = '' OR `+scheduledSendStatusExpr+` = $%[2]d)
			 ORDER BY scheduled_at ASC`, own, statusAt),
			args...)
		if err != nil {
			return fmt.Errorf("scheduled send: listing: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			row, err := scanScheduledSend(rows)
			if err != nil {
				return err
			}
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("scheduled send: reading the list: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.stampReviews(ctx, out)
	return out, nil
}

// stampReviews fills in the review standing over each held message.
//
// AFTER the rows are read rather than joined into the query, because the review
// table is consent's and this module may not reach into it. The seam takes
// every id at once, so a list of thirty messages costs one read.
//
// A LOOKUP FAILURE IS NOT A LIST FAILURE, which is why this returns nothing
// rather than an error. The rows are what the rep asked for and they are
// already correct; a review id is a route to FURTHER work, and failing the
// whole page because a link on it could not be resolved would take away
// something usable to protect something optional.
//
// Unstamped rows read as "no review stands over this" — the honest answer when
// we could not find out, and the same one every unrefused message gives.
func (s *Store) stampReviews(ctx context.Context, rows []ScheduledSend) {
	if s.reviewLookup == nil || len(rows) == 0 {
		return
	}
	// ONLY THE HELD ONES, and this decides the answer rather than merely
	// narrowing the read.
	//
	// A RESCHEDULED MESSAGE IS THE CASE THAT MAKES IT SO. RescheduleInTx moves
	// a held row back to 'scheduled' and deliberately LEAVES ITS REVIEW LIVE —
	// the message is still going out and its decision is still outstanding, and
	// the fire path that carries it opens no review of its own. So the lookup
	// would happily answer for that row, and without this filter a message the
	// rep has already dealt with would carry a route to a refusal about a
	// moment that has passed.
	//
	// It narrows the read too: a list of thirty scheduled messages does not
	// send thirty ids to answer about the two that stopped.
	intents := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		if row.Status == ScheduledStatusHeld {
			intents = append(intents, row.ID)
		}
	}
	if len(intents) == 0 {
		return
	}
	reviews, err := s.reviewLookup.LiveReviewsForIntents(ctx, intents)
	if err != nil {
		// Swallowed for the caller, which is why this returns nothing at all —
		// and LOGGED, because the two are different decisions. A page that
		// silently loses its links every time is indistinguishable from one
		// where no message was refused, so a lost grant or a dead pool would
		// look like ordinary quiet until somebody asked why nobody could reach
		// their reviews.
		slog.WarnContext(ctx, "held messages listed without the reviews standing over them",
			"err", err, "messages", len(intents))
		return
	}
	for i := range rows {
		if review, ok := reviews[rows[i].ID]; ok {
			rows[i].ReviewID, rows[i].ReviewActions = review.ID, review.Actions
		}
	}
}

// GetScheduledSend reads one of the caller's scheduled messages.
func (s *Store) GetScheduledSend(ctx context.Context, id ids.UUID) (ScheduledSend, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ScheduledSend{}, err
	}
	var out ScheduledSend
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = readScheduledSendTx(ctx, tx, id)
		return err
	})
	if err != nil {
		return ScheduledSend{}, err
	}
	one := []ScheduledSend{out}
	s.stampReviews(ctx, one)
	return one[0], nil
}

// readScheduledSendTx is the detail read inside a caller's transaction, under
// the list's own read rule.
func readScheduledSendTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (ScheduledSend, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	own, err := ScheduledSendReadableSQL(ctx, "scheduled_send", arg)
	if err != nil {
		return ScheduledSend{}, err
	}
	query := fmt.Sprintf(`
		SELECT`+scheduledSendColumns+`
		  FROM scheduled_send
		 WHERE id = $%d AND %s`, arg(id), own)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return ScheduledSend{}, fmt.Errorf("scheduled send: reading: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return ScheduledSend{}, fmt.Errorf("scheduled send: reading: %w", err)
		}
		// Existence-hiding: somebody else's scheduled message is not found,
		// never forbidden.
		return ScheduledSend{}, apperrors.ErrNotFound
	}
	return scanScheduledSend(rows)
}

// RescheduleScheduledSend moves a pending message's due moment.
//
// Time only. The content is what the approval bound to, so changing it is
// cancel-and-recompose, which re-enters every gate from the top (ADR-0104 §5).
//
// A HELD message is rescheduled by the same call, and that is the point of the
// held state: it is where a rep recovers from a refusal, having fixed whatever
// caused it. The move clears the hold reason and arms a fresh timer, and the
// gates that refused will be asked again when it fires — a rep who has not
// actually fixed the problem gets held a second time rather than a send.
//
// The expected version is required rather than optional: two surfaces moving
// one message must not silently resolve to whichever wrote last.
func (s *Store) RescheduleScheduledSend(ctx context.Context, id ids.UUID, sched SendSchedule, expectedVersion int64, timer ScheduleTimer) (ScheduledSend, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return ScheduledSend{}, err
	}
	if timer == nil {
		return ScheduledSend{}, errNoScheduleTimer
	}
	sched, err := s.CheckLater(sched)
	if err != nil {
		return ScheduledSend{}, err
	}
	current, err := s.GetScheduledSend(ctx, id)
	if err != nil {
		return ScheduledSend{}, err
	}
	// The version check below refuses a row that was held after this read.
	if current.Status == ScheduledStatusHeld {
		if err := mayReleaseHeld(ctx); err != nil {
			return ScheduledSend{}, err
		}
	}

	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return s.RescheduleInTx(ctx, tx, id, sched, expectedVersion, current, timer)
	}); err != nil {
		return ScheduledSend{}, err
	}
	return s.GetScheduledSend(ctx, id)
}

// RescheduleInTx is RescheduleScheduledSend's write, in the CALLER's transaction.
//
// It exists so a decision releasing this work can consume its approval and do
// the work atomically (approvals.RedeemAndApply): a failed move then leaves the
// approval unconsumed and the card retryable, rather than committing the
// decision and losing the work — which is the exact failure a held-message card
// exists to prevent.
func (s *Store) RescheduleInTx(ctx context.Context, tx pgx.Tx, id ids.UUID, sched SendSchedule, expectedVersion int64, current ScheduledSend, timer ScheduleTimer) error {
	normalized, err := normalizeSchedule(sched)
	if err != nil {
		return err
	}
	sched = normalized
	if unmoved, err := scheduleUnmoved(ctx, tx, id, sched, expectedVersion); err != nil || unmoved {
		return err
	}
	var keptWall *time.Time
	var keptAt time.Time
	{
		// held_reason is cleared with the move: the row is pending again, and a
		// stale reason would have the surface explain a hold that is over. The
		// state-shape CHECK enforces the pairing, so this is not optional.
		err := tx.QueryRow(ctx, `
 UPDATE scheduled_send
 SET (scheduled_wall,scheduled_tz,scheduled_at)=(
  SELECT planned.wall_time,COALESCE(planned.timezone,@zone),planned.instant
  FROM public.update_intent_time(scheduled_wall,scheduled_tz,scheduled_at,@wall::text::timestamp,@zone,@at) AS planned),
			       payload = CASE WHEN held_reason = @replied
			         THEN jsonb_set(payload, '{replies_reviewed_at}', to_jsonb(@reviewed::timestamptz))
			         ELSE payload END,
			       status = 'scheduled', held_reason = NULL,
			       version = version + 1, updated_at = now()
			 WHERE id = @id AND status IN ('scheduled','held') AND version = @version
 RETURNING scheduled_wall,scheduled_at`, pgx.NamedArgs{
			"at": sched.At.UTC(), argZone: sched.TZ, argWall: sched.wallClock(), "id": id, fieldVersion: expectedVersion,
			"replied": HeldRecipientReplied, "reviewed": s.now(),
		}).Scan(&keptWall, &keptAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrVersionSkew
		}
		if err != nil {
			return fmt.Errorf("scheduled send: moving the due moment: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "reschedule", "scheduled_send", id,
			map[string]any{FieldScheduledAt: current.ScheduledAt, FieldScheduledTZ: current.ScheduledTZ, fieldScheduledIntent: current.IntentWire()},
			map[string]any{FieldScheduledAt: keptAt, FieldScheduledTZ: sched.TZ, fieldScheduledIntent: (ScheduledSend{ScheduledWall: keptWall, ScheduledTZ: sched.TZ}).IntentWire()}); err != nil {
			return err
		}
		// A FRESH timer for the new moment. The old one still wakes at the old
		// time and finds a row whose due moment has moved, which it re-snoozes
		// or ignores — the row is the schedule, the job is only an alarm.
		// The rep answered the hold, so its card goes with it.
		if err := s.resolveHeld(ctx, tx, id, HeldResolvedByRep); err != nil {
			return err
		}
		// A review whose decision is already recorded stays, and the fire at
		// the new moment spends it. One nobody decided closes with the card;
		// a refusal at the new moment opens a fresh one.
		if err := s.closeReview(ctx, tx, id, ReviewOutcomeMoved); err != nil {
			return err
		}
		return timer.ScheduleTx(ctx, tx, id, keptAt)
	}
}

// scheduleUnmoved reports a pending send, at the version the caller read, that
// the move would leave due at the moment it already holds. The moment is worked
// out by the same update_intent_time the move uses, so the two agree. A held
// send is never unmoved: moving it releases the hold.
func scheduleUnmoved(ctx context.Context, tx pgx.Tx, id ids.UUID, sched SendSchedule, expectedVersion int64) (bool, error) {
	var unmoved bool
	err := tx.QueryRow(ctx, `
 SELECT s.status = 'scheduled' AND s.version = @version
    AND (s.scheduled_wall, s.scheduled_tz, s.scheduled_at) IS NOT DISTINCT FROM
        (planned.wall_time, COALESCE(planned.timezone, @zone), planned.instant)
   FROM scheduled_send s,
        public.update_intent_time(s.scheduled_wall, s.scheduled_tz, s.scheduled_at, @wall::text::timestamp, @zone, @at) AS planned
  WHERE s.id = @id
    FOR UPDATE OF s`, pgx.NamedArgs{
		"at": sched.At.UTC(), argZone: sched.TZ, argWall: sched.wallClock(), "id": id, fieldVersion: expectedVersion,
	}).Scan(&unmoved)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("scheduled send: comparing the due moment: %w", err)
	}
	return unmoved, nil
}

// CancelScheduledSend withdraws a message before it fires, or gives up on one
// already held.
//
// It does not touch the timer. The job wakes, reads a row that is no longer
// scheduled, and does nothing — which is also what happens if the process dies
// between this write and any attempt to cancel the job, so there is one
// behaviour rather than two.
func (s *Store) CancelScheduledSend(ctx context.Context, id ids.UUID) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	if _, err := s.GetScheduledSend(ctx, id); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return s.CancelInTx(ctx, tx, id)
	})
}

// CancelInTx is CancelScheduledSend's write, in the CALLER's transaction — the
// same reason RescheduleInTx exists.
func (s *Store) CancelInTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	{
		tag, err := tx.Exec(ctx, `
			UPDATE scheduled_send
			   SET status = 'cancelled', held_reason = NULL,
			       version = version + 1, updated_at = now()
			 WHERE id = $1 AND status IN ('scheduled','held')`, id)
		if err != nil {
			return fmt.Errorf("scheduled send: cancelling: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrVersionSkew
		}
		if _, err := storekit.Audit(ctx, tx, "cancel", "scheduled_send", id, nil, nil); err != nil {
			return err
		}
		// Answered, like the reschedule above: a cancelled message needs no
		// decision, so its card must not outlive it.
		if err := s.resolveHeld(ctx, tx, id, HeldResolvedByRep); err != nil {
			return err
		}
		// AND THE REVIEW, if this message was one a refusal froze. Cancelling
		// answers the question that review asks — not by sending, but by
		// deciding not to — and one left live shows a decider work about a
		// message that is already dead.
		//
		// AFTER the message row, which is the lock order every other path
		// takes: the resume claims the scheduled_send FOR UPDATE and then
		// resolves its review. Closing the review first here would take the two
		// locks in the opposite order, and a cancel racing a resume would
		// deadlock — Postgres would abort one of them with a fault neither
		// caller could act on.
		return s.closeReview(ctx, tx, id, ReviewOutcomeCancelled)
	}
}

// closeReview ends the review this message left behind, if a review surface is
// wired and this message had one.
//
// ONE HELPER rather than the same nil check at three call sites, because the
// check is not the interesting part — the OUTCOME is, and a reader comparing
// the three sites should see only that difference between them.
//
// ALWAYS AFTER THE MESSAGE ROW HAS MOVED. That is the lock order every other
// path takes: the resume claims the scheduled_send FOR UPDATE and then reaches
// for its review. Closing the review first would take the two locks in the
// opposite order, and a cancel racing a resume would deadlock — Postgres aborts
// one of them with a fault neither caller can act on.
func (s *Store) closeReview(
	ctx context.Context, tx pgx.Tx, id ids.UUID, outcome ReviewOutcome,
) error {
	if s.reviewCloser == nil {
		return nil
	}
	return s.reviewCloser.CloseReviewForIntentTx(ctx, tx, id, outcome)
}

// resolveHeld clears the inbox card a hold raised, once the rep has acted on
// the message. A surface with no notifier wired has nothing to clear.
func (s *Store) resolveHeld(ctx context.Context, tx pgx.Tx, id ids.UUID, how HeldResolution) error {
	if s.heldNotifier == nil {
		return nil
	}
	return s.heldNotifier.ResolveHeldInTx(ctx, tx, id, how)
}

// scanScheduledSend reads one row, thawing the payload for the fields the
// list and detail surfaces show.
func scanScheduledSend(rows pgx.Rows) (ScheduledSend, error) {
	var (
		row        ScheduledSend
		anchor     *ids.UUID
		payloadRaw []byte
		activityID *ids.UUID
		heldReason *string
		// SQL NULL on a reply row, which the origin-shape CHECK requires: a
		// reply's records come from its anchor, and the ones it adds beyond
		// those travel in also_links, which the fire reads and this does not.
		originLinks     []byte
		additionalLinks []byte
	)
	if err := rows.Scan(
		&row.ID, &row.Status, &row.ScheduledAt, &row.ScheduledTZ, &row.ScheduledWall, &row.OriginKind,
		&anchor, &payloadRaw, &row.ScheduledBy, &activityID,
		&heldReason, &row.Version, &row.CreatedAt, &row.UpdatedAt, &originLinks, &additionalLinks, &row.PayloadVersion, &row.PrincipalKind,
	); err != nil {
		return ScheduledSend{}, fmt.Errorf("scheduled send: reading a row: %w", err)
	}
	// payload_version is not checked on this read, and the fire's refusal of a
	// payload this build did not write is what makes that safe: a stale row can
	// never send, so the worst this read can do is describe a message that will
	// be held — and a rep has to be able to see it to withdraw it.
	var payload scheduledPayload
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		return ScheduledSend{}, fmt.Errorf("scheduled send: reading the frozen message: %w", err)
	}
	links, err := thawOriginLinks(originLinks)
	if err != nil {
		return ScheduledSend{}, err
	}
	row.Links = links
	row.additionalLinks, row.frozen = additionalLinks, payload
	evidence, err := payload.Evidence.thaw()
	if err != nil {
		return ScheduledSend{}, err
	}
	row.Evidence = evidence
	if anchor != nil {
		row.Anchor = ids.ActivityID{UUID: *anchor}
	}
	if activityID != nil {
		row.ActivityID = *activityID
	}
	if heldReason != nil {
		row.HeldReason = *heldReason
	}
	row.Subject = payload.Subject
	row.Recipients = payload.Recipients
	row.Cc = payload.Cc
	row.Bcc = payload.Bcc
	row.Body = payload.Body
	row.Context = commsauthz.Category(payload.Context)
	row.MarketingPurpose = payload.MarketingPurpose
	row.ConsentPurpose = payload.ConsentPurpose
	return row, nil
}
