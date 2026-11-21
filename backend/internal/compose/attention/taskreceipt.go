// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// The kinds of a receipt for a task Margince changed on evidence. Both offer
// the Undo of the audit row that changed it.
const (
	ReceiptKindTaskCompleted = "task_completed"
	ReceiptKindTaskMoved     = "task_moved"
)

// IsTaskReceipt reports whether a receipt kind is a task Margince changed.
func IsTaskReceipt(kind string) bool {
	return kind == ReceiptKindTaskCompleted || kind == ReceiptKindTaskMoved
}

// TaskReceiptKind is the kind of the receipt for one changed task.
func TaskReceiptKind(changed activities.TaskCompletion) string {
	if changed.Moved {
		return ReceiptKindTaskMoved
	}
	return ReceiptKindTaskCompleted
}

// TaskReceiptReason is the sentence saying why the task changed: the mail we
// sent, the meeting booked, or the chat line it rests on.
func TaskReceiptReason(ctx context.Context, changed activities.TaskCompletion) string {
	locale := systemtext.Locale(ctx)
	if changed.Chat {
		words := systemtext.Copy(systemtext.TaskChatKey(changed.Moved), locale)
		if changed.Moved && changed.DueAt != nil {
			return fmt.Sprintf(words, changed.DueAt.Format(time.DateOnly), changed.Quote)
		}
		return fmt.Sprintf(words, changed.Quote)
	}
	contact := changed.Contact
	if contact == "" {
		contact = systemtext.Copy(systemtext.TaskClosedContactKey(changed.Booking), locale)
	}
	return fmt.Sprintf(systemtext.Copy(systemtext.TaskClosedKey(changed.Booking), locale), contact, changed.WrittenAt.Format(time.DateOnly))
}
