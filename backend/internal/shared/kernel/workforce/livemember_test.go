// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workforce

import "testing"

// Asserted literally: a test that rebuilt the pair from the same pieces would
// agree with a change that dropped a half.
func TestLiveMemberSQLNamesBothHalves(t *testing.T) {
	t.Parallel()
	for alias, want := range map[string]string{
		"":   "status = 'active' AND archived_at IS NULL",
		"u":  "u.status = 'active' AND u.archived_at IS NULL",
		"lu": "lu.status = 'active' AND lu.archived_at IS NULL",
	} {
		if got := LiveMemberSQL(alias); got != want {
			t.Errorf("LiveMemberSQL(%q) = %q, want %q", alias, got, want)
		}
	}
}

func TestActivatableMemberSQLAdmitsAnInvitedSeatAndKeepsTheArchivedHalf(t *testing.T) {
	t.Parallel()
	want := "u.status IN ('invited', 'active') AND u.archived_at IS NULL"
	if got := ActivatableMemberSQL("u"); got != want {
		t.Errorf("ActivatableMemberSQL(\"u\") = %q, want %q", got, want)
	}
}

func TestAskableMemberSQLRefusesAnAgent(t *testing.T) {
	t.Parallel()
	want := "u.is_agent = false AND u.status IN ('invited', 'active') AND u.archived_at IS NULL"
	if got := AskableMemberSQL("u"); got != want {
		t.Errorf("AskableMemberSQL(\"u\") = %q, want %q", got, want)
	}
}
