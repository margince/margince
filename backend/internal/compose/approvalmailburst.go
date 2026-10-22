// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One email for a burst of approvals. The sender of a decider's first waiting
// approval also claims their others from the same few minutes
// (notices.ClaimApprovalBurst), and one message names them all, each with its
// own facts and its own link.

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
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
	claimed, err := w.notices.ClaimApprovalBurst(ctx, first, first.Notice.CreatedAt.Add(-approvalMailDelay))
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
// single notice reads exactly as it always has.
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
