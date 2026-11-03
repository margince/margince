// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subjectOrder is the deal ids in the order their tasks are minted.
func subjectOrder(open []assurance.Exception) []ids.UUID {
	var out []ids.UUID
	for _, subject := range groupBySubject(open) {
		out = append(out, subject.id)
	}
	return out
}

// finding is one open finding on a deal, at an amount.
func finding(deal ids.UUID, amount int64) assurance.Exception {
	return assurance.Exception{
		ID: ids.NewV7(), Type: assurance.TypeNoNextStep, SubjectKind: "deal", SubjectID: deal,
		Severity: assurance.SeverityHigh, AffectedMinor: &amount,
	}
}

func TestTheTasksAreMintedInAnOrderTheAmountsDoNotSet(t *testing.T) {
	small, large := ids.NewV7(), ids.NewV7()
	// The read sorts by amount, so each slice lists the bigger deal first.
	smallIsBigger := []assurance.Exception{finding(small, 900_000), finding(large, 1_000)}
	largeIsBigger := []assurance.Exception{finding(large, 900_000), finding(small, 1_000)}

	if a, b := subjectOrder(smallIsBigger), subjectOrder(largeIsBigger); !slices.Equal(a, b) {
		t.Fatalf("swapping the amounts changed the minting order from %v to %v; "+
			"a reader masked from amounts could tell which deal is bigger", a, b)
	}
}

func TestTheMostMaterialFindingStillLeadsItsDeal(t *testing.T) {
	deal := ids.NewV7()
	material, minor := finding(deal, 900_000), finding(deal, 1_000)

	subjects := groupBySubject([]assurance.Exception{material, minor})
	if len(subjects) != 1 || subjects[0].findings[0].ID != material.ID {
		t.Fatalf("the deal's bundle is %+v, want the 900000 finding first", subjects)
	}
}
