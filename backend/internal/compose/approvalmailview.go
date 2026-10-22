// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func refreshApprovalNotice(ctx context.Context, pool *pgxpool.Pool, attempt notices.EmailAttempt, now time.Time) (notices.EmailAttempt, bool, error) {
	if attempt.ApprovalID == nil {
		if attempt.Notice.Kind == notices.KindApprovalPending {
			return attempt, false, errors.New("approval notice has no decision reference")
		}
		return attempt, true, nil
	}
	service := NewApprovalsService(InstallationDB(pool))
	approval, err := service.Get(ctx, *attempt.ApprovalID)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return attempt, false, nil
	}
	if err != nil {
		return attempt, false, err
	}
	if !approval.PendingAt(now) {
		return attempt, false, nil
	}
	current, err := service.GetWire(ctx, *attempt.ApprovalID)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return attempt, false, nil
	}
	if err != nil {
		return attempt, false, err
	}
	if current.RequestedDeciderId == nil || ids.UUID(*current.RequestedDeciderId) != attempt.Recipient.UUID {
		return attempt, false, nil
	}
	if approvals.PreviewRefusal(current.Preview) != nil {
		return attempt, false, nil //nolint:nilerr // A refused preview suppresses mail; delivery itself has not failed.
	}
	if approval.Summary != nil && *approval.Summary != "" {
		attempt.Notice.Subject = *approval.Summary
	}
	if approval.TargetLabel != nil {
		attempt.Notice.Body = mailcopy.OneLine(*approval.TargetLabel)
	}
	if !approvals.ExpiresNever(approval.Kind) {
		words := recipientMailCopy(ctx, pool)
		attempt.Notice.Body += " · " + words.MailDeadline + ": " + words.Instant(recipientDeadline(ctx, pool, approval.ExpiresAt))
	}
	return attempt, true, nil
}

func mailNow(clock func() time.Time) time.Time {
	if clock != nil {
		return clock()
	}
	return time.Now()
}
