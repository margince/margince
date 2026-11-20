// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"encoding/json"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// leadNextTask is the next open task's subject with the descriptor it was
// written with, selected as one row so the two describe one task.
type leadNextTask struct {
	Subject    *string             `json:"subject"`
	SystemText *systemtext.Message `json:"system_text"`
}

// nameNextTask names the lead's next task for the reader: in their language
// when Margince wrote it, as typed when a person did.
func nameNextTask(l *crmcontracts.Lead, raw []byte, reader textlang.Lang) error {
	if len(raw) == 0 {
		return nil
	}
	var task leadNextTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return fmt.Errorf("decode the lead's next task: %w", err)
	}
	if task.Subject != nil {
		l.NextTaskSubject = new(task.SystemText.SubjectIn(*task.Subject, reader))
	}
	return nil
}
