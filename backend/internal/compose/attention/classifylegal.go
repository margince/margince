// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// openedOverdueGrace is how long after its deadline a duty may be recorded and
// still be today's. An Art. 13 duty falls due the moment its contact is
// acquired and is recorded moments later; that is a live breach, not history.
const openedOverdueGrace = 24 * time.Hour

// openedOverdue is a disclosure duty recorded well after it fell due, which is
// what importing old correspondence produces.
func openedOverdue(item crmcontracts.AttentionItem) bool {
	return item.Source == sourceNoticeCase && item.OccurredAt != nil && item.DueAt != nil &&
		item.OccurredAt.Sub(*item.DueAt) > openedOverdueGrace
}

// classifyLegalDeadline ranks both compliance clocks by the same deadline.
// Subject requests remain unassigned; disclosure duties carry the responsible
// officer, falling back to their contact owner. Ownerless duties remain
// available in the unassigned view.
func classifyLegalDeadline(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	// Seven days is the agenda preparation window, not a change to the legal deadline.
	level := levelRoutine
	if item.DueAt != nil && item.DueAt.Sub(asOf) <= 7*24*time.Hour {
		level = levelWaiting
	}
	because := []crmcontracts.WorklistReason{reason("legal_deadline", nil)}
	// A duty recorded after its own deadline came from imported history: it is
	// owed, and reviewed as a backlog rather than ranked as today's breach. The
	// deadline itself is unchanged. A duty that fell due while recorded stays urgent.
	if openedOverdue(item) {
		level = levelRoutine
		because = append(because, reason("opened_overdue", nil))
	}
	row := base(item, level, "system", "legal_deadline_missed")
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = because
	return ranked{
		ownerRef:   ownerFromAssignee(item.AssigneeId),
		owner:      assigneeID(item.AssigneeId),
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}
