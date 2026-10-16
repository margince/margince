// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// Particulars the environment sets are what a message discloses, over the
// row an admin saved. Not parallel: the binding is the process's own.
func TestBootSetParticularsAreWhatAMessageDiscloses(t *testing.T) {
	e := setupChannelConsent(t)
	store := inJurisdiction(t, e, "zb",
		messagingrules.Disclosure{Kind: messagingrules.ControllerIdentity})
	stateParticulars(t, e, ControllerParticulars{LegalName: "Saved GmbH", PostalAddress: "Savedweg 2"})
	bindForTest(t, consentConfig(t,
		`MARGINCE_CONSENT_CONTROLLER_PARTICULARS={"LegalName":"Boot GmbH","PostalAddress":"Bootweg 1"}`))

	lines := disclosures(t, e, store, commsauthz.CategoryReplyToInbound)
	if len(lines) != 1 || lines[0].Text != "Boot GmbH\nBootweg 1" {
		t.Fatalf("the message discloses %+v, want the Boot GmbH the environment sets", lines)
	}
}
