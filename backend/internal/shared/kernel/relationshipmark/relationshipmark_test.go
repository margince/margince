// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relationshipmark

import "testing"

func TestNotMarkedSQLNamesTheAlias(t *testing.T) {
	t.Parallel()
	if got, want := NotMarkedSQL("c"), "c.relationship_mark IS NULL"; got != want {
		t.Fatalf("NotMarkedSQL(c) = %q, want %q", got, want)
	}
}

func TestValidAcceptsOnlyTheVocabulary(t *testing.T) {
	t.Parallel()
	for _, m := range Values() {
		if !Valid(m) {
			t.Errorf("Valid(%q) = false for a listed mark", m)
		}
	}
	for _, bad := range []Mark{"", "vendor", "VENDOR_MARKETING"} {
		if Valid(bad) {
			t.Errorf("Valid(%q) = true, want false", bad)
		}
	}
}

func TestContactNotMarkedSQLNamesTheIDExpression(t *testing.T) {
	t.Parallel()
	want := "EXISTS (SELECT 1 FROM contact mk WHERE mk.id = r.contact_id AND mk.relationship_mark IS NULL)"
	if got := ContactNotMarkedSQL("r.contact_id"); got != want {
		t.Fatalf("ContactNotMarkedSQL = %q, want %q", got, want)
	}
}
