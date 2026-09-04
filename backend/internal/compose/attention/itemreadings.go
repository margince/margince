// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The small readings every classifier shares: what a row's deadline is, when it
// happened, and how long it has been quiet.
//
// Beside classify.go rather than inside it because they answer questions about
// ONE item that every lane asks, while that file decides what each KIND of item
// means. Splitting them keeps each file to one concept, which is also what the
// 500-line cap is asking for.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// stampDeadline marks an item whose date the READER owes.
//
// The flag is what tells a real deadline from a staged proposal's expiry: the
// producers whose date is a deadline resolve it here, and the ones whose date
// is a lapse moment leave it unset, so the header can count work due without
// counting proposals that merely go stale.
func stampDeadline(row *crmcontracts.WorklistItem, due *time.Time, asOf time.Time) {
	if due == nil {
		return
	}
	past := overdueAt(due, asOf)
	row.Overdue = &past
}

func reason(kind crmcontracts.WorklistReasonKind, value *crmcontracts.WorklistValue) crmcontracts.WorklistReason {
	return crmcontracts.WorklistReason{Kind: kind, Value: value}
}

func deadlineOf(due *time.Time) time.Time {
	if due == nil {
		return time.Time{}
	}
	return *due
}

// occurredOf answers when the reported thing happened, falling back to the read
// instant so a row with no timestamp sorts as "now" rather than as 1 January
// year one — which would put every undated row at the head of its level.
func occurredOf(item crmcontracts.AttentionItem, asOf time.Time) time.Time {
	if item.OccurredAt != nil {
		return *item.OccurredAt
	}
	return asOf
}

// quietDaysOf reads the idle count the risk and decay lanes measure.
//
// From the TYPED field, not parsed out of the supporting sentence. It used to
// read `detail` a digit at a time and answer zero for anything else, which made
// the ordering depend on a display string: a lane that made its sentence
// friendlier — "quiet for 90 days" instead of "90" — would have dropped every
// such row to the bottom of the queue, and nothing would have failed.
//
// A lane that measures no idle time sends none, and zero is what that means.
func quietDaysOf(item crmcontracts.AttentionItem) int {
	if item.QuietDays == nil {
		return 0
	}
	return *item.QuietDays
}
