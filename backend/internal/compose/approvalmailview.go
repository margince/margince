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

// approvalNoticeUse says who reads the refreshed notice. Only the single email
// reads an approval's facts. The digest is claimed once and not retried, so a
// failed read of them must not cost the seat all of it.
type approvalNoticeUse bool

const (
	forDigestLine  approvalNoticeUse = false
	forSingleEmail approvalNoticeUse = true
)

// refreshApprovalNotice re-reads the approval as the recipient. The notice's
// body stays the line the digest shows; the facts are what the single email
// adds.
func refreshApprovalNotice(
	ctx context.Context, pool *pgxpool.Pool, attempt notices.EmailAttempt, now time.Time, use approvalNoticeUse,
) (notices.EmailAttempt, approvalMailFacts, bool, error) {
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
	// The decider is in the conversation that staged it and sees it there. Their
	// approvals list and digest still carry it.
	if use == forSingleEmail && approval.StagedInItsDecidersSession() {
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
	if approvals.PreviewRefusal(current.Preview) != nil && !awaitsWiderSight(current.Preview) {
		return attempt, facts, false, nil //nolint:nilerr // A refused preview suppresses mail; delivery itself has not failed.
	}
	return withApprovalWords(ctx, pool, attempt, current, approval.ExpiresAt, use)
}

// withApprovalWords writes the notice's subject and digest line for the
// recipient, and the facts its single email adds. A commitment has its own;
// any other approval says what its preview shows.
func withApprovalWords(
	ctx context.Context, pool *pgxpool.Pool, attempt notices.EmailAttempt, current crmcontracts.Approval, expires time.Time, use approvalNoticeUse,
) (notices.EmailAttempt, approvalMailFacts, bool, error) {
	var facts approvalMailFacts
	// A digest line is all its reader gets, so it keeps the values; the single
	// email lists them under the sentence.
	if line := digestOrSubjectLine(current, use); line != "" {
		attempt.Notice.Subject = line
	}
	if current.TargetLabel != nil {
		attempt.Notice.Body = mailcopy.OneLine(*current.TargetLabel)
	}
	words := recipientMailCopy(ctx, pool)
	local := func(at time.Time) time.Time { return recipientDeadline(ctx, pool, at) }
	switch {
	case use != forSingleEmail:
	case current.Kind == CommitmentTaskKind:
		var err error
		if facts, err = newApprovalPreviewReader(InstallationDB(pool)).commitmentMailFacts(ctx, current, words, local); err != nil {
			return attempt, facts, false, err
		}
	case current.Preview != nil:
		facts = changeMailFacts(current, words)
	}
	if !approvals.ExpiresNever(current.Kind) {
		facts.Deadline = words.MailDeadline + ": " + words.Instant(local(expires))
		attempt.Notice.Body += " · " + facts.Deadline
	}
	return attempt, facts, true, nil
}

// awaitsWiderSight is a preview the recipient cannot decide because its effect
// touches records they cannot open. The card tells them who must decide it, and
// so does the email, rather than leave the request unannounced.
func awaitsWiderSight(preview *crmcontracts.ApprovalPreview) bool {
	return preview != nil && preview.State == approvals.PreviewNeedsWiderSight
}

func mailNow(clock func() time.Time) time.Time {
	if clock != nil {
		return clock()
	}
	return time.Now()
}

// digestOrSubjectLine is the joined summary for a digest line and the sentence
// alone for the single email's subject.
func digestOrSubjectLine(approval crmcontracts.Approval, use approvalNoticeUse) string {
	if use == forSingleEmail {
		return approvalSentence(approval)
	}
	if approval.Summary != nil {
		return *approval.Summary
	}
	return ""
}
