// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// bookedDiscoverClause renders the reader's own discover gate for the booked
// meeting's alias. It fences what a waiting row may SHOW. Every eligibility
// clause beside it stays reader-independent. The three statements that render
// waitingRepliesSQL share it, each beside its own argument list.
func bookedDiscoverClause(ctx context.Context, arg func(any) int) (string, error) {
	return auth.ActivityDiscoverClause(ctx, "booked", arg)
}

// requestTaskDiscoverClause is the same reader's discover gate for the request
// reminder that may stand in for a waiting message. A reminder the reader
// cannot find must not hide the message from them.
func requestTaskDiscoverClause(ctx context.Context, arg func(any) int) (string, error) {
	return auth.ActivityDiscoverClause(ctx, "request_task", arg)
}
