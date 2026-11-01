// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMemberAuthorshipBindsItsPatternOutsideTheSQL(t *testing.T) {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	clause := memberAuthorshipSQL("a", arg(ids.Nil), arg)
	if len(args) != 2 || args[1] != "%:" {
		t.Fatalf("authorship arguments = %v", args)
	}
	if strings.Contains(clause, "%") || !strings.Contains(clause, "a.captured_by LIKE ANY (ARRAY(SELECT $2 || author_identity.id::text") {
		t.Fatalf("authorship interpolates its pattern into SQL: %s", clause)
	}
}
