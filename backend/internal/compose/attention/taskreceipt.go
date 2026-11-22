// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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
	if changed.Evidence == activities.EvidenceChat || changed.Moved {
		return chatReason(changed, locale)
	}
	booking := changed.Evidence == activities.EvidenceBooking
	contact := changed.Contact
	if contact == "" {
		contact = systemtext.Copy(systemtext.TaskClosedContactKey(booking), locale)
	}
	return fmt.Sprintf(systemtext.Copy(systemtext.TaskClosedKey(booking), locale), contact, changed.WrittenAt.Format(time.DateOnly))
}

// chatReason names the chat line a change rests on when the reader may read
// the chat, and the chat alone otherwise. A move always names its new day,
// which the move itself wrote.
func chatReason(changed activities.TaskCompletion, locale textlang.Lang) string {
	quoted := changed.Quote != ""
	key := systemtext.TaskChatUnquotedKey(changed.Moved)
	if quoted {
		key = systemtext.TaskChatKey(changed.Moved)
	}
	words := systemtext.Copy(key, locale)
	var args []any
	if changed.Moved {
		day := ""
		if changed.DueAt != nil {
			day = changed.DueAt.Format(time.DateOnly)
		}
		args = append(args, day)
	}
	if quoted {
		args = append(args, changed.Quote)
	}
	return fmt.Sprintf(words, args...)
}
