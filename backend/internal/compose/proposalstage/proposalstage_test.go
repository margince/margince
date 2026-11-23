// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package proposalstage

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
)

func TestOnlyOurOwnOutboundEmailCanAnnounceAProposal(t *testing.T) {
	t.Parallel()
	ours := deals.ActivityAuthorship{Kind: "email", Direction: "outbound", AuthorSide: deals.AuthorSeller}
	if !MailEligible(ours) {
		t.Fatal("our own outbound email was refused")
	}
	refused := map[string]func(deals.ActivityAuthorship) deals.ActivityAuthorship{
		"inbound mail":       func(a deals.ActivityAuthorship) deals.ActivityAuthorship { a.Direction = "inbound"; return a },
		"written by a buyer": func(a deals.ActivityAuthorship) deals.ActivityAuthorship { a.AuthorSide = deals.AuthorBuyer; return a },
		"side unknown": func(a deals.ActivityAuthorship) deals.ActivityAuthorship {
			a.AuthorSide = deals.AuthorUnknown
			return a
		},
		"a meeting": func(a deals.ActivityAuthorship) deals.ActivityAuthorship { a.Kind = "meeting"; return a },
	}
	for name, change := range refused {
		if MailEligible(change(ours)) {
			t.Errorf("%s was admitted", name)
		}
	}
}
