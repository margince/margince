// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"
	"testing"
)

// The fragment must compare through a guarded cast: an unguarded
// `::uuid` raises on the first malformed source_id and fails the whole
// statement, and a text comparison is what forced the activity table scan.
func TestCitedActivityIDCastsOnlyACanonicalUUID(t *testing.T) {
	got := CitedActivityID("cite")
	for _, want := range []string{
		"CASE WHEN (cite->>'source_id') ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'",
		"THEN (cite->>'source_id')::uuid END",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("CitedActivityID(cite) = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "::text") {
		t.Fatalf("CitedActivityID must not compare as text: %q", got)
	}
}
