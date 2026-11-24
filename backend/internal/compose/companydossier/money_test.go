// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import "testing"

func TestTheDossierKeepsASumItsFieldStatesAndDropsAnInventedOne(t *testing.T) {
	in := fourOfSeven()
	in.ProfileFields[0].Value = "Load-shifting software, 180,000 EUR a year per site"
	field := in.ProfileFields[0].Id.String()
	cases := map[string]int{"A site pays 180,000 EUR a year.": 1, "A site pays 950,000 EUR a year.": 0}
	for text, want := range cases {
		reply := cited(in, `{"sections":[{"kind":"summary","sentences":[{"text":"`+text+`","nature":"fact","evidence":[{"entity_type":"profile_field","entity_id":"`+field+`"}]}]}]}`)
		sections, err := ParseDossier(reply, in)
		if err != nil || len(sections) != want {
			t.Errorf("%q kept %d sections (%v), want %d", text, len(sections), err, want)
		}
	}
}
