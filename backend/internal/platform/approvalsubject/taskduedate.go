// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// KindTaskDueDate asks the owner of a captured task whether to keep a due
// date far outside the allowed window.
const KindTaskDueDate = "task_due_date"

// TaskDueDate is that question. OwnerID decides it. DueDate starts as the
// captured day and is what the owner keeps, changes or clears.
type TaskDueDate struct {
	TaskID     ids.UUID `json:"task_id"`
	OwnerID    ids.UUID `json:"owner_id"`
	Summary    string   `json:"summary"`
	DueDate    string   `json:"due_date"`
	OddDueDate string   `json:"odd_due_date"`
}
