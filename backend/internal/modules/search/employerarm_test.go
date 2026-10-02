// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"
	"strings"
	"testing"
)

// A seat missing any of the three grants gets no arm, and binds nothing: a
// parameter no statement references would fail the whole search it rides.
func TestARefusedEmployerArmBindsNothing(t *testing.T) {
	t.Parallel()
	for _, missing := range []string{entityContact, entityCompany, objectRelationship} {
		var objects []string
		for _, object := range []string{entityContact, entityCompany, objectRelationship} {
			if object != missing {
				objects = append(objects, object)
			}
		}
		var bound []any
		arm, err := employerArmSQL(teamReaderFor(objects...), 1, 2, true, func(v any) int { bound = append(bound, v); return len(bound) })
		if err != nil {
			t.Fatalf("without %s: %v", missing, err)
		}
		if arm != "" || len(bound) != 0 {
			t.Errorf("without %s the arm is %q with %d argument(s) bound, want none", missing, arm, len(bound))
		}
	}
}

// An admitted arm references every argument it bound, and carries the one
// currency rule rather than its own.
func TestAnAdmittedEmployerArmUsesWhatItBinds(t *testing.T) {
	t.Parallel()
	bound := []any{"", "acme"}
	arm, err := employerArmSQL(teamReaderFor(entityContact, entityCompany, objectRelationship), 1, 2, true,
		func(v any) int { bound = append(bound, v); return len(bound) })
	if err != nil {
		t.Fatal(err)
	}
	for i := range bound {
		if !strings.Contains(arm, fmt.Sprintf("$%d", i+1)) {
			t.Errorf("parameter $%d is bound and never used", i+1)
		}
	}
	if !strings.Contains(arm, "coalesce(r.employment_status, 'current')") {
		t.Error("the arm no longer reads employment through employment.IsCurrentSQL")
	}
}

func TestAConjunctionKeepsEachClauseWhole(t *testing.T) {
	t.Parallel()
	if got := conjunction("", ""); got != "true" {
		t.Errorf("no clauses rendered %q, want true", got)
	}
	if got := conjunction("a OR b", "", "c"); got != "(a OR b) AND (c)" {
		t.Errorf("rendered %q", got)
	}
}
