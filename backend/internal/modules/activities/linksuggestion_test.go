// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A proposal names a unit or the one colleague it asks, never both or neither.
// So a unit cannot hide its suggestion from every reader but one.
func TestAProposalNamesItsUnitOrItsColleague(t *testing.T) {
	colleague := ids.NewV7()
	for name, tc := range map[string]struct {
		unit    string
		forUser ids.UUID
		valid   bool
	}{
		"a unit":      {unit: "finance-demo", valid: true},
		"a colleague": {forUser: colleague, valid: true},
		"both":        {unit: "finance-demo", forUser: colleague},
		"neither":     {},
	} {
		in := LinkSuggestionInput{EntityType: "deal", EntityID: ids.NewV7(), Unit: tc.unit, ForUser: tc.forUser, Reason: "r", Score: 0.5}
		err := validateLinkSuggestion(&in)
		if tc.valid && err != nil {
			t.Errorf("a proposal naming %s was refused: %v", name, err)
		}
		if !tc.valid && !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("a proposal naming %s answered %v, want invalid argument", name, err)
		}
	}
}
