// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The staging refusal names the cause the review ranks first, not the first
// recipient the engine decided about. A log or a client without the review
// then reads the same first refusal as the composer.
func TestStagingNamesTheRefusalTheReviewRanksFirst(t *testing.T) {
	t.Parallel()
	refusal := func(address string, verdict commsauthz.Verdict, code string) commsauthz.Decision {
		return commsauthz.Decision{
			Recipient: connector.Recipient{Email: address}, Verdict: verdict, ReasonCode: code, Mode: commsauthz.ModeEnforce,
		}
	}
	err := StagingRefusal(commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		refusal("unproven@a.test", commsauthz.VerdictReview, commsauthz.ReasonNoEvidence),
		refusal("objected@a.test", commsauthz.VerdictDeny, commsauthz.ReasonObjection),
		refusal("bounced@a.test", commsauthz.VerdictDeny, commsauthz.ReasonHardBounce),
	}})
	if !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("StagingRefusal = %v, want a consent refusal", err)
	}
	if want := "(" + commsauthz.ReasonHardBounce + ")"; !strings.Contains(err.Error(), want) {
		t.Errorf("refusal %q does not name %s", err.Error(), want)
	}
}
