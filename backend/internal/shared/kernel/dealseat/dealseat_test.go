// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealseat

import (
	"strings"
	"testing"
)

func TestCurrentSQLAsksEveryColumnOfTheNamedAlias(t *testing.T) {
	t.Parallel()
	got := CurrentSQL("seat")
	for _, want := range []string{"seat.kind = 'deal_stakeholder'", "seat.archived_at IS NULL", "seat.ended_at IS NULL OR seat.ended_at > "} {
		if !strings.Contains(got, want) {
			t.Errorf("CurrentSQL(seat) = %q, want it to contain %q", got, want)
		}
	}
}

func TestCurrentSQLWithoutAnAliasNamesBareColumns(t *testing.T) {
	t.Parallel()
	got := CurrentSQL("")
	if strings.Contains(got, ".kind") || !strings.HasPrefix(got, "(kind = ") {
		t.Fatalf("CurrentSQL(\"\") = %q, want unqualified columns", got)
	}
}
