// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The follow-up statements read their row, a later contact and a reply that
// lifts a snooze. Postgres plans every copy of the content gate a statement
// carries, and a pooled connection keeps that memory, so each statement gates
// those activities once. These tests pin it.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// statementTx keeps the statement a read sends and fails it, so the read is
// rendered without a database.
type statementTx struct {
	pgx.Tx
	sql string
}

var errRendered = errors.New("rendered")

func (tx *statementTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	tx.sql = sql
	return nil, errRendered
}

func TestTheAwaitingRepliesStatementCarriesOneContentGate(t *testing.T) {
	tx := &statementTx{}
	if _, err := (&Store{}).queryAwaitingReplies(teamScopedCtx(), tx, clocktest.Now(t), 3, nil); !errors.Is(err, errRendered) {
		t.Fatalf("rendering the awaiting-replies read: %v", err)
	}
	if got := auth.ContentGateCopies(tx.sql); got != 1 {
		t.Fatalf("the awaiting-replies read carries %d content gates, want 1", got)
	}
}

func TestTheMeetingFollowUpStatementCarriesOneContentGate(t *testing.T) {
	everyMeeting := func(func(any) int) (string, error) { return scopeUnbounded, nil }
	sql, _, err := meetingFollowUpsStatement(teamScopedCtx(), clocktest.Now(t), 3, everyMeeting, AwaitingReplyScanCap)
	if err != nil {
		t.Fatalf("rendering the meeting follow-up read: %v", err)
	}
	if got := auth.ContentGateCopies(sql); got != 1 {
		t.Fatalf("the meeting follow-up read carries %d content gates, want 1", got)
	}
}
