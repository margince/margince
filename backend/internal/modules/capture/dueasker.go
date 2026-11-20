// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DueAsker puts a captured task's due date outside the allowed window to the
// colleague who owns the task. Compose injects the approvals engine.
type DueAsker interface {
	AskDueTx(ctx context.Context, tx pgx.Tx, question DueQuestion) error
}

// DueQuestion names the task and the day that was captured for it.
type DueQuestion struct {
	TaskID  ids.ActivityID
	OwnerID ids.UUID
	Summary string
	Due     time.Time
}

// WithDueAsker returns a copy that asks about out-of-window due dates.
func (s *Sink) WithDueAsker(asker DueAsker) *Sink {
	c := *s
	c.dueAsker = asker
	return &c
}
