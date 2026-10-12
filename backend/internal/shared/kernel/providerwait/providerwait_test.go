// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package providerwait_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/providerwait"
)

// The predicates splice Detail into SQL as a literal, so a quote in the text
// would end that literal early and change what the predicate selects.
func TestDetailCanBeSplicedIntoALiteral(t *testing.T) {
	if strings.ContainsAny(providerwait.Detail, `'\`) {
		t.Fatalf("Detail %q holds a quote or backslash", providerwait.Detail)
	}
}

func TestPredicatesAreBuiltFromDetail(t *testing.T) {
	for name, clause := range map[string]string{"Clause": providerwait.Clause, "NotClause": providerwait.NotClause} {
		if !strings.HasSuffix(clause, "'"+providerwait.Detail+"'") {
			t.Errorf("%s = %q does not end in the literal Detail", name, clause)
		}
	}
}

// A row with no detail waits on the budget, so NotClause must keep it. A plain
// `<>` would be NULL for that row and drop it from the budget count.
func TestNotClauseKeepsARowWithNoDetail(t *testing.T) {
	if !strings.Contains(providerwait.NotClause, "IS DISTINCT FROM") {
		t.Fatalf("NotClause = %q, which would drop a row whose status_detail is NULL", providerwait.NotClause)
	}
}
