// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The draft prompt is told the recipient's buying role only when it is a
// built-in one, so the model cannot write "as our contact" from a typed value.
func TestTheDraftPromptCarriesOnlyABuiltInBuyingRole(t *testing.T) {
	for stored, want := range map[string]string{
		"contact": "", "Key contact": "", " Champion ": "champion", "economic_buyer": "economic_buyer",
	} {
		var in Input
		role := stored
		foldCommercial(&in, crmcontracts.Contact360{Commercial: &crmcontracts.Contact360Commercial{Role: &role}})
		if in.Recipient.BuyingRole != want {
			t.Errorf("%q: buying role = %q, want %q", stored, in.Recipient.BuyingRole, want)
		}
	}
}
