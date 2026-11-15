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
