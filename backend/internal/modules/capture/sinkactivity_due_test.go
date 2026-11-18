// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// dueArgOf is the due_at argument of the capture insert, found by its column.
func dueArgOf(t *testing.T, due, now time.Time) *time.Time {
	t.Helper()
	statement, args := capturedActivityInsert(context.Background(), connector.NormalizedRecord{}, ActivityFields{Kind: "task", DueAt: due}, birthDecision{}, now)
	columns := strings.Split(statement[strings.Index(statement, "(")+1:strings.Index(statement, ")")], ",")
	for at, column := range columns {
		if column == "due_at" {
			got, ok := args[at].(*time.Time)
			if !ok {
				t.Fatalf("due_at argument is %T, want *time.Time", args[at])
			}
			return got
		}
	}
	t.Fatal("the capture insert has no due_at column")
	return nil
}

func TestTheCaptureInsertFilesNoDueDateOutsideTheWindowAndKeepsTheEdge(t *testing.T) {
	now := clocktest.Now(t)
	if got := dueArgOf(t, time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), now); got != nil {
		t.Errorf("a 1900 due date was stored as %v", got)
	}
	last := now.AddDate(values.DueYearsAhead, 0, 0)
	if got := dueArgOf(t, last, now); got == nil || !got.Equal(last) {
		t.Errorf("the last day inside the window was stored as %v, want %v", got, last)
	}
	if got := dueArgOf(t, last.Add(time.Second), now); got != nil {
		t.Errorf("one second past the window was stored as %v", got)
	}
	if got := dueArgOf(t, time.Time{}, now); got != nil {
		t.Errorf("no due date was stored as %v", got)
	}
}
