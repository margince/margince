// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// refreshApprovalNotice re-reads the approval as the recipient. The notice's
// body stays the line the digest shows; the facts are what the single email
// adds, and only a commitment has any.
func refreshApprovalNotice(ctx context.Context, pool *pgxpool.Pool, attempt notices.EmailAttempt, now time.Time) (notices.EmailAttempt, approvalMailFacts, bool, error) {
	var facts approvalMailFacts
	if attempt.ApprovalID == nil {
		if attempt.Notice.Kind == notices.KindApprovalPending {
			return attempt, facts, false, errors.New("approval notice has no decision reference")
		}
		return attempt, facts, true, nil
	}
	service := NewApprovalsService(InstallationDB(pool))
	approval, err := service.Get(ctx, *attempt.ApprovalID)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return attempt, facts, false, nil
	}
	if err != nil {
		return attempt, facts, false, err
	}
	if !approval.PendingAt(now) {
		return attempt, facts, false, nil
	}
	current, err := service.GetWire(ctx, *attempt.ApprovalID)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return attempt, facts, false, nil
	}
	if err != nil {
		return attempt, facts, false, err
	}
	if current.RequestedDeciderId == nil || ids.UUID(*current.RequestedDeciderId) != attempt.Recipient.UUID {
		return attempt, facts, false, nil
	}
	if approvals.PreviewRefusal(current.Preview) != nil {
		return attempt, facts, false, nil //nolint:nilerr // A refused preview suppresses mail; delivery itself has not failed.
	}
	return withApprovalWords(ctx, pool, attempt, current, approval.ExpiresAt)
}

// withApprovalWords writes the notice's subject and digest line for the
// recipient, and the facts a commitment's single email adds.
func withApprovalWords(
	ctx context.Context, pool *pgxpool.Pool, attempt notices.EmailAttempt, current crmcontracts.Approval, expires time.Time,
) (notices.EmailAttempt, approvalMailFacts, bool, error) {
	var facts approvalMailFacts
	if current.Summary != nil && *current.Summary != "" {
		attempt.Notice.Subject = *current.Summary
	}
	if current.TargetLabel != nil {
		attempt.Notice.Body = mailcopy.OneLine(*current.TargetLabel)
	}
	words := recipientMailCopy(ctx, pool)
	local := func(at time.Time) time.Time { return recipientDeadline(ctx, pool, at) }
	if current.Kind == CommitmentTaskKind {
		var err error
		if facts, err = newApprovalPreviewReader(InstallationDB(pool)).commitmentMailFacts(ctx, current, words, local); err != nil {
			return attempt, facts, false, err
		}
	}
	if !approvals.ExpiresNever(current.Kind) {
		facts.Deadline = words.MailDeadline + ": " + words.Instant(local(expires))
		attempt.Notice.Body += " · " + facts.Deadline
	}
	return attempt, facts, true, nil
}

func mailNow(clock func() time.Time) time.Time {
	if clock != nil {
		return clock()
	}
	return time.Now()
}
