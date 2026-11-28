// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatwire

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// RootRef keeps source-root identity and revision evidence together.
type RootRef struct {
	ID        string
	Revision  string
	UpdatedAt time.Time
	Deleted   bool
}

// RootPage carries discovery continuation separately from completed scan coverage.
type RootPage struct {
	Roots      []RootRef
	Next       string
	Checkpoint string
}

// ThreadPage distinguishes pagination from confirmed source deletion.
type ThreadPage struct {
	Revision string
	Messages []Message
	Next     string
	Deleted  bool
}

// RootReader provides canonical thread state for edit and deletion reconciliation.
type RootReader interface {
	Roots(context.Context, string, time.Time, string) (RootPage, error)
	Thread(context.Context, string, string, string) (ThreadPage, error)
	Audience(context.Context, string) (messaging.Conversation, error)
}
