// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One email for a burst of approvals. The sender of a decider's first waiting
// approval also claims their others from the same few minutes
// (notices.ClaimApprovalBurst). One message names them all, each with its own
// facts and its own link.

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// approvalMailDelay is how long an approval's email waits before it is sent,
// and how far back its burst reaches.
const approvalMailDelay = 3 * time.Minute

// approvalMailItem is one notice a message carries, as refreshed for the reader.
type approvalMailItem struct {
	attempt notices.EmailAttempt
	facts   approvalMailFacts
}

// approvalBurst claims and refreshes the decider's other waiting approval
// notices. A notice that is no longer theirs to decide is spent and dropped,
// as its own job would have dropped it. A failed claim takes nothing, so their
// own jobs still send them.
func (w *notificationMailWorker) approvalBurst(ctx, seatCtx context.Context, first notices.EmailAttempt) []approvalMailItem {
	claimed, err := w.notices.ClaimApprovalBurst(ctx, first, approvalMailDelay)
	if err != nil {
		w.log.WarnContext(ctx, "the decider's other approvals were left to their own messages",
			"notice", first.Notice.ID, "cause", err)
		return nil
	}
	var items []approvalMailItem
	for _, attempt := range claimed {
		refreshed, facts, current, err := refreshApprovalNotice(seatCtx, w.pool, attempt, mailNow(w.now), forSingleEmail)
		if err != nil {
			w.recordMailFailure(ctx, attempt.Notice.ID, "approval could not be read")
			continue
		}
		if current {
			items = append(items, approvalMailItem{attempt: refreshed, facts: facts})
		}
	}
	return items
}

// approvalMailMessage is the subject and message for what one send carries. A
// single notice reads as it always has.
func approvalMailMessage(items []approvalMailItem, base string, words mailcopy.Copy) (string, emailview.Message) {
	if len(items) == 1 {
		item := items[0]
		message := notificationMessage(item.attempt.Notice, item.facts, notificationBase(item.attempt, base), words)
		if item.attempt.ApprovalID != nil {
			message.AppendContent(approvalMailAction(item.attempt, base, words))
		}
		return notificationMailSubject(item.attempt.Notice, words), message
	}
	var body emailview.Content
	body.Paragraph(words.NotificationIntro)
	for _, item := range items {
		writeNoticeSection(&body, item.attempt.Notice, item.facts)
		body.Append(approvalMailAction(item.attempt, base, words))
	}
	body.Action(words.MailReview, mailcopy.Destination(base, mailcopy.WorklistFragment))
	subject := words.NotificationSubject + fmt.Sprintf(words.MailBurstCount, len(items))
	return subject, body.Message(subject, string(words.Language), words.NotificationPreview)
}

// reserveApprovalMail reads the decisions the message carries through the
// shared send basis, as the recipient, and holds that basis just before the
// relay call. It keeps those still waiting on them, with their promises open
// and their source readable. The words kept are the ones read at the versions
// held.
func (w *notificationMailWorker) reserveApprovalMail(ctx, seatCtx context.Context, items []approvalMailItem, words mailcopy.Copy) []approvalMailItem {
	kept, won, err := reserveSend(seatCtx, w.pool, w.approvalReading(seatCtx, items, words), reserveInOwnTx[[]approvalMailItem](seatCtx, w.pool))
	if err != nil {
		w.failApprovalMail(ctx, items, "the decision changed or could not be rechecked before the send")
		return nil
	}
	if !won {
		return nil
	}
	return kept
}

// approvalReading renders the claimed notices as the recipient, leaving out
// the decisions excluded and the ones no longer theirs. Two readings agree
// when they make the same message.
func (w *notificationMailWorker) approvalReading(seatCtx context.Context, items []approvalMailItem, words mailcopy.Copy) sendReading[[]approvalMailItem] {
	clock := func() time.Time { return mailNow(w.now) }
	render := func(excluded map[ids.UUID]bool) ([]approvalMailItem, sendRefs, bool, error) {
		var kept []approvalMailItem
		var refs sendRefs
		for _, item := range items {
			if item.attempt.ApprovalID != nil && excluded[item.attempt.ApprovalID.UUID] {
				continue
			}
			refreshed, facts, current, err := refreshApprovalNotice(seatCtx, w.pool, item.attempt, clock(), forSingleEmail)
			if err != nil {
				return nil, refs, false, err
			}
			if !current {
				continue
			}
			if refreshed.ApprovalID != nil {
				refs.approvals = append(refs.approvals, refreshed.ApprovalID.UUID)
			}
			kept = append(kept, approvalMailItem{attempt: refreshed, facts: facts})
		}
		return kept, refs, len(kept) > 0, nil
	}
	same := func(a, b []approvalMailItem) bool {
		subjectA, messageA := approvalMailMessage(a, w.mail.PublicBaseURL, words)
		subjectB, messageB := approvalMailMessage(b, w.mail.PublicBaseURL, words)
		return subjectA == subjectB && messageA.Text == messageB.Text
	}
	return sendReading[[]approvalMailItem]{seat: items[0].attempt.Recipient.UUID, clock: clock, render: render, same: same}
}

func (w *notificationMailWorker) failApprovalMail(ctx context.Context, items []approvalMailItem, cause string) {
	for _, item := range items {
		w.recordMailFailure(ctx, item.attempt.Notice.ID, cause)
	}
}
