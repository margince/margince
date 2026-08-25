// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "testing"

// The predicate is a string the whole tree now depends on, so it is asserted
// literally rather than rebuilt from the same pieces the code uses — a test
// that composed it the same way would agree with any change, including one
// that dropped a half.
func TestLiveMemberSQLNamesBothHalves(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alias string
		want  string
	}{
		{"unaliased, for a query reading app_user directly", "", "status = 'active' AND archived_at IS NULL"},
		{"aliased, for a query that joins app_user", "u", "u.status = 'active' AND u.archived_at IS NULL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := LiveMemberSQL(tc.alias); got != tc.want {
				t.Errorf("LiveMemberSQL(%q) = %q, want %q", tc.alias, got, tc.want)
			}
		})
	}
}
