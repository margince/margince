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

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noticeKindAutomation labels a notice an automation's notify action raised.
const noticeKindAutomation = "automation"

// noticeKindLeadSLA labels the lead-SLA escalation notice.
const noticeKindLeadSLA = "lead_sla"

// noticesNotifier adapts the notices store onto automation's Notifier seam.
// The recipient arrives from the automation's own decoded arguments; the
// store's FK is what refuses a recipient who is not a live seat, and the
// principal (the engine's system identity) is what captured_by records — the
// notice never impersonates anyone.
type noticesNotifier struct{ store *notices.Store }

func (n noticesNotifier) Notify(ctx context.Context, recipient ids.UUID, subject, body string) error {
	_, err := n.store.Create(ctx, ids.From[ids.UserKind](recipient), noticeKindAutomation, subject, body)
	return err
}
