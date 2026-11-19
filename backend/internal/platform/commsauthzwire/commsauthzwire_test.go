// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthzwire

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestAFindingCarriesItsLawAndAnUncitedOneNone(t *testing.T) {
	t.Parallel()
	law := commsauthz.Provision{Instrument: "UWG", Provision: "§ 7(3)", SourceURL: "https://example.org/uwg"}
	wired := *FooterFindings([]commsauthz.FooterFinding{
		{Kind: "objection_route", Provision: law},
		{Kind: "recipient_private_link"},
	})
	if cited := wired[0].Provision; cited == nil || cited.Provision != "§ 7(3)" || cited.SourceUrl != law.SourceURL {
		t.Errorf("the cited finding renders %+v, want %+v", cited, law)
	}
	if wired[1].Provision != nil || wired[1].MissingMeans == nil {
		t.Errorf("the uncited finding renders %+v, want no provision and an empty means list", wired[1])
	}
}
