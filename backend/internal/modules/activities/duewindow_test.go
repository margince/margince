// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestADueDateOutsideTheWindowIsRefusedOnDueAt(t *testing.T) {
	now := clocktest.Now(t)
	cases := map[string]struct {
		due     time.Time
		refused bool
	}{
		"year 1900":               {time.Date(1900, 1, 1, 9, 30, 0, 0, time.UTC), true},
		"year 9999":               {time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), true},
		"just over ten years ago": {now.AddDate(-10, 0, -1), true},
		"just past ten years":     {now.AddDate(10, 0, 1), true},
		"two years ago":           {now.AddDate(-2, 0, 0), false},
		"next month":              {now.AddDate(0, 1, 0), false},
		"nine years ahead":        {now.AddDate(9, 0, 0), false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := checkDueWindow(&c.due, now)
			if !c.refused {
				if err != nil {
					t.Fatalf("checkDueWindow refused %v: %v", c.due, err)
				}
				return
			}
			fault, ok := errors.AsType[apperrors.FieldFault](err)
			if !ok {
				t.Fatalf("checkDueWindow answered %v, want a field fault", err)
			}
			if field, _, _ := fault.FieldFault(); field != "due_at" {
				t.Fatalf("fault names %q, want due_at", field)
			}
		})
	}
}

func TestNoDueDatePassesTheWindow(t *testing.T) {
	if err := checkDueWindow(nil, clocktest.Now(t)); err != nil {
		t.Fatalf("checkDueWindow(nil) = %v", err)
	}
}
