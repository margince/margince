// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// cited is a dossier reply scripted with record ids as the model writes it:
// each id the request showed spelled as its ref. An id the request never
// showed stays as written, which is how a model that invents one answers.
func cited(in Input, reply string) string { return dossierShown(in, "").Refer(reply) }

func parseDossier(reply string, in Input) ([]Section, error) {
	return ParseDossier(cited(in, reply), in)
}

// The dossier prompt shows each profile field and fact by its ref. A reply
// citing a ref reads back to the record's id; the id itself, and a ref the
// request never minted, ground nothing.
func TestTheDossierShowsRefsAndReadsThemBackToRecords(t *testing.T) {
	in := fourOfSeven()
	if shown := DossierRequest(in, string(textlang.German)).Messages[0].Content; claims.SpellsRecordID(shown) {
		t.Fatalf("the request shows a record id:\n%s", shown)
	}
	field := in.ProfileFields[0].Id.String()
	for ref, want := range map[string]int{"pf1": 1, field: 0, "pf99": 0} {
		reply := `{"sections":[{"kind":"summary","sentences":[{"text":"They build load-shifting software.","nature":"fact",` +
			`"evidence":[{"entity_type":"profile_field","entity_id":"` + ref + `"}]}]}]}`
		sections, err := ParseDossier(reply, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(sections) != want || (want == 1 && sections[0].Sentences[0].Evidence[0].EntityID != field) {
			t.Errorf("citing %q kept %+v, want %d section(s) citing the field's id", ref, sections, want)
		}
	}
}
