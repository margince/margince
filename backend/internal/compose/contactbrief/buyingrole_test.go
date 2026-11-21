// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// A typed buying role reaches neither the prompt nor the deal line. A built-in
// one, however it was capitalised, is named in the reader's language.
func TestOnlyABuiltInBuyingRoleIsNamed(t *testing.T) {
	cases := []struct {
		stored, language, wantInput, wantRole string
	}{
		{"contact", "en", "", "with no buying role recorded"},
		{"Key contact", "de", "", "ohne erfasste Rolle"},
		{" Champion ", "de", "champion", "Erfasst als Fürsprecher"},
		{"economic_buyer", "en", "economic_buyer", "recorded economic buyer"},
	}
	for _, c := range cases {
		in := inputFixture()
		stored := c.stored
		foldCommercial(&in, crmcontracts.Contact360{Commercial: &crmcontracts.Contact360Commercial{Role: &stored}})
		if in.BuyingRole != c.wantInput {
			t.Errorf("%q: prompt input role = %q, want %q", c.stored, in.BuyingRole, c.wantInput)
		}
		in.OpenDeal = inputFixture().OpenDeal
		line := dealLine(in, langcopy.For(c.language))
		if !strings.Contains(line, c.wantRole) || strings.Contains(strings.ToLower(line), "contact") {
			t.Errorf("%q/%s: line = %q, want it to contain %q", c.stored, c.language, line, c.wantRole)
		}
	}
}
