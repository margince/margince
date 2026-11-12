// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capturehealth reads the per-mailbox, sender-queue and review counts
// behind the capture-health page. Counts and ages only, never correspondence.
package capturehealth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// UTCOrNil is at in UTC, or nil when there is none.
func UTCOrNil(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}

// MailboxHealth counts what is waiting per mailbox owner.
//
// A full join of two independent backlogs, so a mailbox in either is reported.
// A seat with contacts waiting and no held threads is as stuck as one with both.
//
// Contacts count as owner-private rows with no settled answer in the sender
// ledger. The promotion reads the same two facts, and the owner's own list the
// same clause, so the count is what each owner is shown.
func MailboxHealth(
	ctx context.Context, tx pgx.Tx, now time.Time,
) ([]crmcontracts.CaptureMailboxHealth, error) {
	rows, err := tx.Query(ctx, `
		WITH waiting_contacts AS (
		  SELECT p.owner_id AS user_id, count(*) AS n, min(p.created_at) AS oldest
		    FROM contact p
		   WHERE `+capture.AwaitingSenderDecisionClause("p")+`
		   GROUP BY p.owner_id),
		unread_periods AS (
		  SELECT user_id, min(unread_from) AS unread_from, max(unread_to) AS unread_to
		    FROM capture_connection
		   WHERE unread_from IS NOT NULL AND user_id IS NOT NULL
		   GROUP BY user_id),
		waiting_threads AS (
		  SELECT v.user_id, count(*) AS n, min(v.created_at) AS oldest
		    FROM capture_thread_verdict v
		   WHERE v.status = 'pending' AND v.attempts >= `+fmt.Sprint(capture.PendingMaxAttempts)+` AND EXISTS (SELECT 1 FROM activity m WHERE `+capture.HeldMessageSQL("v", "m")+`)
		   GROUP BY v.user_id)
		SELECT coalesce(c.user_id, t.user_id, g.user_id),
		       u.display_name,
		       coalesce(c.n, 0), c.oldest,
		       coalesce(t.n, 0), t.oldest,
		       g.unread_from, g.unread_to
		  FROM waiting_contacts c
		  FULL JOIN waiting_threads t ON t.user_id = c.user_id
		  FULL JOIN unread_periods g ON g.user_id = coalesce(c.user_id, t.user_id)
		  LEFT JOIN app_user u ON u.id = coalesce(c.user_id, t.user_id, g.user_id)
		 ORDER BY coalesce(c.n, 0) + coalesce(t.n, 0) DESC,
		          coalesce(c.user_id, t.user_id, g.user_id)`)
	if err != nil {
		return nil, fmt.Errorf("capturehealth: reading what each mailbox is waiting on: %w", err)
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
			unreadFrom, unreadTo   *time.Time
		)
		if err := rows.Scan(&userID, &name, &contacts, &oldestContact, &threads, &oldestT,
			&unreadFrom, &unreadTo); err != nil {
			return nil, err
		}
		row := crmcontracts.CaptureMailboxHealth{
			UserId:                   openapi_types.UUID(userID),
			ContactsAwaitingDecision: contacts,
			ThreadsAwaitingVerdict:   threads,
			OldestContactAgeSeconds:  AgeSeconds(now, oldestContact),
			OldestThreadAgeSeconds:   AgeSeconds(now, oldestT),
		}
		row.DisplayName = name
		row.UnreadPeriod = unreadPeriodOf(unreadFrom, unreadTo)
		out = append(out, row)
	}
	return out, rows.Err()
}

// ClassifierHealth counts the sender queue across every mailbox.
//
// `exhausted` is apart from `unsure`. Both need a human, but one is the machine
// unable to tell and the other the machine having stopped asking. With no model
// everything retires to `unsure`, so a large `unsure` beside a small `pending`
// says the classifier is not running.
func ClassifierHealth(
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
			"capturehealth: reading the sender queue: %w", err)
	}
	out.OldestPendingAgeSeconds = AgeSeconds(now, oldestPending)
	return out, nil
}

// unreadPeriodOf is the gap a mailbox was never read for, or nil when there is
// none to report.
//
// Both ends or neither, which the column CHECK holds. Read for it anyway. A
// half period on the wire is one a client cannot choose a backfill window
// from, and this is where that shape would reach it.
func unreadPeriodOf(from, to *time.Time) *crmcontracts.CaptureUnreadPeriod {
	if from == nil || to == nil {
		return nil
	}
	return &crmcontracts.CaptureUnreadPeriod{From: from.UTC(), To: to.UTC()}
}

// AgeSeconds renders how long something has been waiting, or absent when
// nothing is. Absent and zero differ: one is an empty queue, the other a queue
// that just filled.
func AgeSeconds(now time.Time, since *time.Time) *int {
	if since == nil {
		return nil
	}
	seconds := max(int(now.UTC().Sub(since.UTC()).Seconds()), 0)
	return &seconds
}

// OwnerReviews lists the mailbox owners with decisions ready for review.
func OwnerReviews(ctx context.Context, tx pgx.Tx) ([]crmcontracts.CaptureOwnerReview, error) {
	rows, err := tx.Query(ctx, `SELECT r.user_id, u.display_name, r.sender_decisions, r.thread_decisions
 FROM (`+capture.OwnerReviewCountsSQL()+`) r LEFT JOIN app_user u ON u.id=r.user_id ORDER BY (r.sender_decisions+r.thread_decisions) DESC, u.display_name, r.user_id`)
	if err != nil {
		return nil, fmt.Errorf("read mailbox decision owners: %w", err)
	}
	defer rows.Close()
	out := []crmcontracts.CaptureOwnerReview{}
	for rows.Next() {
		var owner crmcontracts.CaptureOwnerReview
		if err := rows.Scan(&owner.UserId, &owner.DisplayName, &owner.SenderDecisions, &owner.ThreadDecisions); err != nil {
			return nil, fmt.Errorf("reading mailbox review owner: %w", err)
		}
		out = append(out, owner)
	}
	return out, rows.Err()
}
