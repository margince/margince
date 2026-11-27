// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package taskexplain

import "github.com/margince/margince/backend/internal/compose/claims"

// stockPhrases are what a sentence says when it would fit any task. A reply
// that leans on one has explained nothing, so the sentence is dropped.
var stockPhrases = []string{
	// English
	"stays undone", "remains undone", "mark it done", "mark it as done", "mark the task",
	"move it to another day", "finish the task", "complete the task", "do the task",
	"the task will not get done", "the task won't get done", "has no due date",
	// German
	"bleibt liegen", "bleibt unerledigt", "als erledigt markieren", "auf einen anderen tag",
	"erledige die aufgabe", "aufgabe abschließen",
	// Vietnamese
	"chưa được hoàn thành", "đánh dấu hoàn thành", "chuyển sang ngày khác", "hoàn thành công việc",
}

// Generic says whether a sentence is stock copy rather than about this task.
func Generic(text string) bool {
	return claims.SaysStock(text, stockPhrases)
}
