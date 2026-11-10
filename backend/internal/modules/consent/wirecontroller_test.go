// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// The page names the controller by the rule a mail's disclosure line uses. So
// a name with no address is shown as an identity on neither.
func TestThePageAndTheMailAgreeOnWhatNamesTheController(t *testing.T) {
	t.Parallel()
	if got := wireController(ControllerParticulars{LegalName: "Beispiel GmbH"}); got != nil {
		t.Errorf("a name with no address is shown as %+v, which the mail would report missing", got)
	}
	stated := ControllerParticulars{LegalName: "Beispiel GmbH", PostalAddress: "Hauptstr. 1", PrivacyContact: "privacy@beispiel.test"}
	got := wireController(stated)
	if got == nil || got.Identity == nil || *got.Identity != particularFor(messagingrules.ControllerIdentity, stated) {
		t.Fatalf("the page names %+v, want the mail's identity line", got)
	}
	if got.PrivacyContact == nil || *got.PrivacyContact != "privacy@beispiel.test" || got.ObjectionRoute != nil {
		t.Errorf("the page names contact %v and route %v, want only what was stated", got.PrivacyContact, got.ObjectionRoute)
	}
}
