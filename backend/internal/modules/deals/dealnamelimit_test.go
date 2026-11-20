// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// A deal name longer than its column is the caller's to shorten. It is refused
// at the field before any write, not by the database after one.
func TestADealNameOverItsLimitIsRefusedAtTheField(t *testing.T) {
	long := strings.Repeat("n", values.TextLabelCharacters+1)
	s := &Store{}

	_, createErr := s.readyDealCreate(systemFilterCtx(), CreateDealInput{Name: long})
	_, updateErr := s.dealUpdatePatch(systemFilterCtx(), nil, crmcontracts.Deal{Name: "Before"},
		UpdateDealInput{Name: &long})

	for name, err := range map[string]error{"create": createErr, "update": updateErr} {
		t.Run(name, func(t *testing.T) { faulttest.AssertNamesField(t, err, dealNameColumn) })
	}
}

func TestADealNameAtItsLimitIsAccepted(t *testing.T) {
	name := strings.Repeat("n", values.TextLabelCharacters)
	if _, err := (&Store{}).readyDealCreate(systemFilterCtx(), CreateDealInput{Name: name}); err != nil {
		t.Fatalf("a name of exactly the limit was refused: %v", err)
	}
}
