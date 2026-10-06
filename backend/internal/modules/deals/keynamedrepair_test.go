// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import "testing"

func TestAKeyNamedDealTakesItsSourceTitleThenItsCompanyAndStage(t *testing.T) {
	cases := []struct {
		name, key, title, company, stage, want string
		ok                                     bool
	}{
		{"the export's title wins", "acme-q3", "Acme Q3 renewal", "Acme", "Proposal", "Acme Q3 renewal", true},
		{"a blank title falls back", "acme-q3", "  ", "Acme", "Proposal", "Acme · Proposal", true},
		{"a title that is the key is no title", "acme-q3", "acme-q3", "Acme", "Proposal", "Acme · Proposal", true},
		{"no title and no company is left for a user", "acme-q3", "", "", "Proposal", "", false},
		{"a fallback that is the key is no rename", "Acme · Proposal", "", "Acme", "Proposal", "", false},
		{"the fallback is one plain line", "acme-q3", "", " Acme\x1b[2K Corp\n", "Pro\tposal", "Acme[2K Corp · Pro posal", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := keyNameReplacement(c.key, c.title, c.company, c.stage)
			if got != c.want || ok != c.ok {
				t.Errorf("keyNameReplacement = (%q, %v), want (%q, %v)", got, ok, c.want, c.ok)
			}
		})
	}
}
