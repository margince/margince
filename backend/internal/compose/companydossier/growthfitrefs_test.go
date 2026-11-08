// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// firstFieldRef is the ref the growth-fit prompt shows for an input's first
// profile field.
const firstFieldRef = "pf1"

func TestTheGrowthFitPromptNamesRecordsByRefAndTheReplyReadsBack(t *testing.T) {
	in := sevenOfSeven()
	content := GrowthFitRequest(in, string(textlang.English)).Messages[0].Content
	for _, field := range in.ProfileFields {
		if strings.Contains(content, field.Id.String()) {
			t.Fatalf("the prompt shows the profile field id %s:\n%s", field.Id, content)
		}
	}
	if !strings.Contains(content, `"id":"`+firstFieldRef+`"`) {
		t.Fatalf("the prompt does not name the first profile field %s:\n%s", firstFieldRef, content)
	}

	_, kept, err := ParseGrowthFit(citing("strong", in), in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := kept.Evidence(); len(got) != 1 || got[0].EntityID != in.ProfileFields[0].Id.String() {
		t.Errorf("evidence = %+v, want the first profile field's id", got)
	}
}

func TestAGrowthFitCitationOfARefNeverShownOrARawIDGroundsNothing(t *testing.T) {
	in := sevenOfSeven()
	for _, cited := range []string{"pf99", "fa1x", in.ProfileFields[0].Id.String()} {
		reply := `{"band":"strong","positive_factors":[{"text":"They sell software.","nature":"fact",
			"evidence":[{"entity_type":"profile_field","entity_id":"` + cited + `"}]}]}`
		if _, _, err := ParseGrowthFit(reply, in); err == nil {
			t.Errorf("a claim citing %q was kept", cited)
		}
	}
}

func TestAGrowthFitClaimSpellingARefInItsTextIsDropped(t *testing.T) {
	in := sevenOfSeven()
	reply := `{"band":"strong","positive_factors":[{"text":"Per pf1 they sell software.","nature":"fact",
		"evidence":[{"entity_type":"profile_field","entity_id":"pf1"}]}]}`
	if _, _, err := ParseGrowthFit(reply, in); err == nil {
		t.Error("a claim spelling its ref in reader text was kept")
	}
}
