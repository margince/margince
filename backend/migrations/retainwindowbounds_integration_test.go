// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"
)

// The database refuses a retention window outside its bounds, whoever writes it.
//
// The store validates retain_days, and that guard binds its own callers and
// nobody else. A migration, a support session or a second writer reaches the
// column directly.
//
// A zero erases records the moment they are created. A value past two million
// days makes the nightly pass answer "timestamp out of range" instead of
// acting.
func TestARetentionWindowOutsideItsBoundsIsRefusedByTheDatabase(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		days int
		want bool
	}{
		{name: "a zero window acts on creation", days: 0},
		{name: "a negative window", days: -1},
		{name: "past the century ceiling", days: 36501},
		{name: "the largest int32, which the pass cannot evaluate", days: 2147483647},
		{name: "one day", days: 1, want: true},
		{name: "the ceiling itself", days: 36500, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := conn.Exec(ctx, `
				INSERT INTO retention_policy (object_type, category, retain_days, action)
				VALUES ('contact', $1, $2, 'archive')`, tc.name, tc.days)
			if tc.want {
				if err != nil {
					t.Fatalf("the database refused a legitimate %d-day window: %v", tc.days, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("the database accepted a %d-day window, so the store is its only guard", tc.days)
			}
			if !strings.Contains(err.Error(), "retention_policy_retain_days_check") {
				t.Fatalf("a %d-day window was refused by something other than its own CHECK: %v", tc.days, err)
			}
		})
	}
}
