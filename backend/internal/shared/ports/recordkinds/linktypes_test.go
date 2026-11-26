// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordkinds

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestCoreLinkTypesNeedNoUnit(t *testing.T) {
	Reset()
	for _, name := range []string{"follow_up_of", "renews", "related_to"} {
		d, ok := LookupLinkType(name)
		if !ok || d.Unit != "" || d.Label == "" {
			t.Errorf("LookupLinkType(%q) = %+v, %v", name, d, ok)
		}
	}
}

func TestAnExtensionLinkTypeIsFoundUnderItsUnitsName(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	RegisterLinkTypes("alpha", []extension.LinkType{
		{Type: "follow_up_of", Label: "Follow-up", From: []string{"deal"}, To: []string{"deal"}},
	})
	d, ok := LookupLinkType("ext:alpha:follow_up_of")
	if !ok || d.Unit != "alpha" {
		t.Fatalf("lookup = %+v, %v", d, ok)
	}
	if _, ok := LookupLinkType("ext:beta:follow_up_of"); ok {
		t.Error("another unit's name resolved")
	}
	if !d.Allows("deal", "deal") || d.Allows("deal", "contact") || d.Allows("company", "deal") {
		t.Error("Allows ignores the declared ends")
	}
	if all := LinkTypes(); len(all) != 4 || all[0].Name != "follow_up_of" || all[3].Name != "ext:alpha:follow_up_of" {
		t.Errorf("LinkTypes() = %+v, want core's three then the unit's", all)
	}
}

func TestACoreLinkTypeJoinsAnyTwoOfTheThreeRecords(t *testing.T) {
	d, _ := LookupLinkType("related_to")
	for _, from := range extension.LinkEnds {
		for _, to := range extension.LinkEnds {
			if !d.Allows(from, to) {
				t.Errorf("related_to refuses %s to %s", from, to)
			}
		}
	}
}
