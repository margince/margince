// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Introductions returns asks addressed to the reader. Wider record scopes do
// not authorize reading another colleague's introduction requests.
type Introductions interface {
	Pending(ctx context.Context, limit int) ([]PendingIntroduction, error)
}

// PendingIntroduction carries the target identity, not its display name.
// The shared label resolver checks the reader's grants before showing a name.
type PendingIntroduction struct {
	ID        ids.UUID
	ContactID ids.UUID
	// Reason is the requester's own sentence for why the ask is worth making,
	// written by them at the time. Never composed here: this queue puts no
	// words in a colleague's mouth.
	Reason      string
	RequestedAt time.Time
	// DueAt is when the ask lapses. It is what this lane orders by, so the one
	// about to expire is the one a reader sees first.
	DueAt time.Time
}
