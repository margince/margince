// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// The composer offers a recorded reason where the preview says a refusal can be
// overruled. So the preview never promises one the record would refuse.
func TestThePreviewOffersAnOverrideOnlyWhereARecordedReasonIsAccepted(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		commsauthz.ReasonNoEvidence:                     true,
		commsauthz.ReasonNoMarketingConsent:             true,
		commsauthz.ReasonLegacyTransactionalUnevidenced: true,
		commsauthz.ReasonUnknownPurpose:                 false,
		commsauthz.ReasonClaimContradictsResolution:     false,
		commsauthz.ReasonObjection:                      false,
	}
	for code, want := range cases {
		decision := commsauthz.Decision{
			Verdict:    commsauthz.VerdictDeny,
			ReasonCode: code,
			Mode:       commsauthz.ModeEnforce,
		}
		out := previewResponse(commsauthz.DecisionSet{Decisions: []commsauthz.Decision{decision}})
		got := out.Recipients[0].CanBeOverruled
		if got == nil {
			t.Fatalf("%s: can_be_overruled is absent", code)
		}
		if *got != want {
			t.Errorf("%s: can_be_overruled = %v, want %v", code, *got, want)
		}
	}
}
