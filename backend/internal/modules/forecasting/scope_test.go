// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Which scopes may be WRITTEN against, as opposed to merely reported.

import "testing"

// managed_teams resolves from an omitted READ scope — a manager's teams and
// themselves — and names no single subject. A forecast recorded against it
// would be an assertion about a population nobody can name, and the standing
// call for it could never be looked up again, so the write door refuses it.
func TestManagedTeamsIsRefusedAsAWriteScope(t *testing.T) {
	if err := checkScope(Scope{Kind: ScopeManagedTeams}); err == nil {
		t.Fatal("a forecast was accepted against the managed-teams population")
	}
}
