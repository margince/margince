// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workforce

import "testing"

func TestPassportUnexpiredSQLReadsANullExpiryAsLive(t *testing.T) {
	got := PassportUnexpiredSQL("p", "now()")
	want := `(p.expires_at IS NULL OR now() < p.expires_at)`
	if got != want {
		t.Fatalf("PassportUnexpiredSQL = %q, want %q", got, want)
	}
}
