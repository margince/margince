// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The queries behind the capture-health page, held apart from the gate in front
// of them.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readCaptureHealth answers what every mailbox is waiting on, and the sender
// queue as a whole.
//
// One transaction, so the per-mailbox rows and the classifier totals describe
// the same instant. Read separately they would drift by however long the first
// took, and an administrator comparing them would be told the difference is a
// backlog.
func readCaptureHealth(ctx context.Context, tx pgx.Tx, now time.Time) (crmcontracts.CaptureHealth, error) {
	out := crmcontracts.CaptureHealth{GeneratedAt: now.UTC()}
	mailboxes, err := captureMailboxHealth(ctx, tx, now)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.Mailboxes = mailboxes
	classifier, err := captureClassifierHealth(ctx, tx, now)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.Classifier = classifier
	held, err := captureHeldMeetings(ctx, tx, now)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.HeldMeetings = held
	sweeps, err := captureSweepHealth(ctx, tx)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.Sweeps = sweeps
	return out, nil
}

// captureHeldMeetings counts what the nightly pass has still to lift, by the
// pass's own selector and without its per-tick bound.
func captureHeldMeetings(
	ctx context.Context, tx pgx.Tx, now time.Time,
) (crmcontracts.CaptureHeldMeetings, error) {
	var (
		args   []any
		out    crmcontracts.CaptureHeldMeetings
		oldest *time.Time
	)
	arg := func(v any) int { args = append(args, v); return len(args) }
	if err := tx.QueryRow(ctx, `
		SELECT count(*), min(a.created_at)
		  FROM activity a
		 WHERE `+filedMeetingHeldClause("a", arg), args...).Scan(&out.Count, &oldest); err != nil {
		return crmcontracts.CaptureHeldMeetings{}, fmt.Errorf(
			"compose: counting the filed meetings still held: %w", err)
	}
	out.OldestAgeSeconds = ageSeconds(now, oldest)
	return out, nil
}

// sweepJobKinds names the job that runs each pass, for its declared cadence.
var sweepJobKinds = map[capture.Sweep]string{
	capture.SweepSettledThreadVerdicts: ConfidentialityVerdictArgs{}.Kind(),
	capture.SweepStrandedContacts:      LinkReconcileArgs{}.Kind(),
	capture.SweepFiledMeetingHolds:     LinkReconcileArgs{}.Kind(),
}

// captureSweepHealth reports every pass's latest receipt and last success.
func captureSweepHealth(ctx context.Context, tx pgx.Tx) ([]crmcontracts.CaptureSweepHealth, error) {
	states, err := capture.SweepStatesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.CaptureSweepHealth, 0, len(states))
	for _, state := range states {
		row := crmcontracts.CaptureSweepHealth{
			Sweep:           crmcontracts.CaptureSweepHealthSweep(state.Sweep),
			LastSucceededAt: utcOrNil(state.LastSucceededAt),
		}
		if spec, ok := jobs.SpecFor(sweepJobKinds[state.Sweep]); ok && spec.Cadence.Fixed > 0 {
			seconds := int(spec.Cadence.Fixed.Seconds())
			row.CadenceSeconds = &seconds
		}
		if last := state.Latest; last != nil {
			run := crmcontracts.CaptureSweepRun{
				Outcome:    crmcontracts.CaptureSweepRunOutcome(last.Outcome),
				FinishedAt: last.FinishedAt.UTC(),
				Processed:  last.Processed,
				CapHit:     last.CapHit,
			}
			if class := vettedSweepClass(last.ErrorClass); class != "" {
				run.ErrorClass = &class
			}
			row.LastRun = &run
		}
		out = append(out, row)
	}
	return out, nil
}

func utcOrNil(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}

// captureMailboxHealth counts what is waiting per mailbox owner.
//
// A FULL JOIN of two independent backlogs, so a mailbox appearing in either is
// reported: a seat with contacts waiting and no held threads is exactly as
// stuck as one with both, and an inner join would hide it.
//
// Contacts are counted as owner-private rows with no settled answer in the
// sender ledger. That is the same pair of facts the promotion reads — a contact
// stays `owner` until a verdict widens it — and the same clause the owner's own
// list reads, so the count is exactly what each owner is shown.
func captureMailboxHealth(
	ctx context.Context, tx pgx.Tx, now time.Time,
) ([]crmcontracts.CaptureMailboxHealth, error) {
	rows, err := tx.Query(ctx, `
		WITH waiting_contacts AS (
		  SELECT p.owner_id AS user_id, count(*) AS n, min(p.created_at) AS oldest
		    FROM contact p
		   WHERE `+capture.AwaitingSenderDecisionClause("p")+`
		   GROUP BY p.owner_id),
		waiting_threads AS (
		  SELECT v.user_id, count(*) AS n, min(v.created_at) AS oldest
		    FROM capture_thread_verdict v
		   WHERE v.status IN ('pending', 'unsure')
		   GROUP BY v.user_id)
		SELECT coalesce(c.user_id, t.user_id),
		       u.display_name,
		       coalesce(c.n, 0), c.oldest,
		       coalesce(t.n, 0), t.oldest
		  FROM waiting_contacts c
		  FULL JOIN waiting_threads t ON t.user_id = c.user_id
		  LEFT JOIN app_user u ON u.id = coalesce(c.user_id, t.user_id)
		 ORDER BY coalesce(c.n, 0) + coalesce(t.n, 0) DESC, coalesce(c.user_id, t.user_id)`)
	if err != nil {
		return nil, fmt.Errorf("compose: reading what each mailbox is waiting on: %w", err)
	}
	defer rows.Close()

	// Empty rather than nil: the wire field is a required array, and null
	// would make a client refuse the whole report on a calm installation.
	out := []crmcontracts.CaptureMailboxHealth{}
	for rows.Next() {
		var (
			userID                 ids.UUID
			name                   *string
			contacts, threads      int
			oldestContact, oldestT *time.Time
		)
		if err := rows.Scan(&userID, &name, &contacts, &oldestContact, &threads, &oldestT); err != nil {
			return nil, err
		}
		row := crmcontracts.CaptureMailboxHealth{
			UserId:                   openapi_types.UUID(userID),
			ContactsAwaitingDecision: contacts,
			ThreadsAwaitingVerdict:   threads,
			OldestContactAgeSeconds:  ageSeconds(now, oldestContact),
			OldestThreadAgeSeconds:   ageSeconds(now, oldestT),
		}
		row.DisplayName = name
		out = append(out, row)
	}
	return out, rows.Err()
}

// captureClassifierHealth counts the sender queue across every mailbox.
//
// `exhausted` is separate from `unsure` on purpose: both need a human, but one
// is the machine saying it cannot tell and the other is the machine having
// stopped asking. An installation with no model retires everything to `unsure`,
// so a large `unsure` beside a small `pending` says the classifier is not
// running rather than that the mail is hard — which is the reading an
// administrator most needs and cannot get from a single total.
func captureClassifierHealth(
	ctx context.Context, tx pgx.Tx, now time.Time,
) (crmcontracts.CaptureClassifierHealth, error) {
	var (
		out           crmcontracts.CaptureClassifierHealth
		oldestPending *time.Time
	)
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'pending' AND next_attempt_at IS NOT NULL),
		       count(*) FILTER (WHERE status = 'unsure'),
		       count(*) FILTER (WHERE status = 'pending' AND next_attempt_at IS NULL),
		       min(created_at) FILTER (WHERE status = 'pending')
		  FROM capture_pending_counterparty`).
		Scan(&out.Pending, &out.Unsure, &out.Exhausted, &oldestPending); err != nil {
		return crmcontracts.CaptureClassifierHealth{}, fmt.Errorf(
			"compose: reading the sender queue: %w", err)
	}
	out.OldestPendingAgeSeconds = ageSeconds(now, oldestPending)
	return out, nil
}

// ageSeconds renders how long something has been waiting, or absent when
// nothing is. Absent and zero are different answers — one is an empty queue,
// the other a queue that just filled — and a page reading zero as empty would
// report the busiest moment as the calmest.
func ageSeconds(now time.Time, since *time.Time) *int {
	if since == nil {
		return nil
	}
	seconds := max(int(now.UTC().Sub(since.UTC()).Seconds()), 0)
	return &seconds
}
