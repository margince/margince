// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestCanonicalRecoveryPreservesArgumentsAndTranslatedMeaning(t *testing.T) {
	for _, component := range []*Component{Sentence("lead.reply", Label("Lena")), Sentence("agent.bulkChange", Count(3)), Sentence("agent.setUpPartner", Text("Company"), Text("reseller"))} {
		original := Canonical(component, textlang.English)
		recovered, err := RecoverCanonicalText(original)
		if err != nil {
			t.Fatal(err)
		}
		if recovered == nil || Canonical(recovered, textlang.Vietnamese) != Canonical(component, textlang.Vietnamese) {
			t.Fatalf("cannot recover %q as %s", original, component.Code)
		}
	}
	if recovered, err := RecoverCanonicalText("My own instructions for Lena"); err != nil || recovered != nil {
		t.Fatalf("human prose recovered as %+v: %v", recovered, err)
	}
}

func TestTheBookingTitleRecoversToItsCodeInEveryEnglishWording(t *testing.T) {
	definition, _ := definitionOf("booking.meeting")
	for _, title := range storedWordings(definition, textlang.English) {
		recovered, err := RecoverCanonicalText(title)
		if err != nil || recovered == nil || recovered.Code != "booking.meeting" {
			t.Errorf("%q recovered as %+v, not booking.meeting: %v", title, recovered, err)
		}
	}
}

func BenchmarkRecoverCanonicalText(b *testing.B) {
	for name, text := range map[string]string{
		"template": Canonical(Sentence("lead.reply", Label("Lena")), textlang.English),
		"prose":    "My own instructions for Lena",
	} {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				if _, err := RecoverCanonicalText(text); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
