// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The notification transport: a durable notice row with its own read-state.
// Recording the row IS the delivery, which is what lets an engine record a
// notify action successful the moment Notify returns without claiming a
// channel this repo does not have — the exact honesty the nil-Notifier skip
// used to protect.

import (
	"context"

	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noticeKindAutomation labels a notice an automation's notify action raised.
const noticeKindAutomation = "automation"

// noticeKindLeadSLA labels the lead-SLA escalation notice.
const noticeKindLeadSLA = "lead_sla"

// noticeKindCaptureBacklog labels the notice raised when a seat's capture
// backlog stops moving.
const noticeKindCaptureBacklog = "capture_backlog_stalled"

// noticesNotifier adapts the notices store onto automation's Notifier seam.
// The recipient arrives from the automation's own decoded arguments; the
// store's FK is what refuses a recipient who is not a live seat, and the
// principal (the engine's system identity) is what captured_by records — the
// notice never impersonates anyone.
type noticesNotifier struct{ store *notices.Store }

// No dedupe key, and that is the honest answer here rather than an omission:
// the Notifier seam is handed a recipient and two strings, and nothing in it
// names the event the notice is about. An automation that re-delivers writes a
// second line, which is the same behaviour it had before the key existed —
// closing it means the seam carrying an identity, not this adapter inventing
// one out of the words.
func (n noticesNotifier) Notify(ctx context.Context, recipient ids.UUID, subject, body string) error {
	_, err := n.store.Create(ctx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](recipient),
		Kind:      noticeKindAutomation,
		Subject:   subject,
		Body:      body,
	})
	return err
}

// NewBacklogStallNotifier raises the capture-backlog notice for one seat.
//
// Straight to notices.Store.Create rather than through the Notifier seam,
// because this caller HAS a natural key for the event and the seam has no way to
// carry one. The key names the seat and the day: a stall is a standing condition
// and the pass that finds it runs hourly, so without a key it writes one line
// per sweep forever, and with a key that named only the seat it would write one
// line ever and go unnoticed on the second week.
func NewBacklogStallNotifier(store *notices.Store, clock func() time.Time) BacklogNotifier {
	return func(ctx context.Context, seat ids.UUID, waiting int) error {
		_, err := store.Create(ctx, notices.NewNotice{
			Recipient: ids.From[ids.UserKind](seat),
			Kind:      noticeKindCaptureBacklog,
			Subject:   "Your captured mail is waiting on the classifier",
			Body: fmt.Sprintf(
				"%d sender decisions have been waiting without an answer. Mail stays withheld from "+
					"your colleagues while this lasts — nothing is lost, and it clears on its own "+
					"once the classifier answers again.", waiting),
			DedupeKey: fmt.Sprintf("capture_backlog_stalled:%s:%s",
				seat, clock().UTC().Format(time.DateOnly)),
		})
		return err
	}
}
