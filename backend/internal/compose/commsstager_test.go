// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What staging refuses before a message is queued.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The four refusals no rollout mode may soften stop the send HERE, not two
// phases later in a worker.
//
// Observe mode lets every other disagreement through on purpose: the engine is
// being measured, and blocking on a difference nobody has reviewed would refuse
// legitimate mail. These four are not differences of opinion — transmit already
// refuses them, so staging them buys nothing and costs the rep the chance to
// learn now.
func TestStagingRefusesTheDenialsNoModeMaySoften(t *testing.T) {
	for _, reason := range []string{
		commsauthz.ReasonObjection,
		commsauthz.ReasonRestricted,
		commsauthz.ReasonHardBounce,
		commsauthz.ReasonUnconfirmedDOI,
		commsauthz.ReasonNoSubject,
		commsauthz.ReasonConsentWithdrawn,
	} {
		set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
			{Verdict: commsauthz.VerdictDeny, ReasonCode: reason},
		}}
		err := refuseAbsoluteDenial(set)
		if !errors.Is(err, apperrors.ErrConsentNotGranted) {
			t.Errorf("%s staged anyway: err = %v", reason, err)
		}
	}
}

// And the converse, or the test above would pass with every send refused: an
// ordinary disagreement is recorded and let through, which is what observe mode
// is for.
func TestStagingLetsAnObservedDisagreementThrough(t *testing.T) {
	set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		{Verdict: commsauthz.VerdictReview, ReasonCode: commsauthz.ReasonNoEvidence},
	}}
	if err := refuseAbsoluteDenial(set); err != nil {
		t.Errorf("refuseAbsoluteDenial = %v, want nil while the engine is only being observed", err)
	}
}

// A message every recipient may receive is not refused for any reason.
func TestStagingAllowsAnOrdinarySend(t *testing.T) {
	set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		{Verdict: commsauthz.VerdictAllow, ReasonCode: commsauthz.ReasonAllowed},
	}}
	if err := refuseAbsoluteDenial(set); err != nil {
		t.Errorf("refuseAbsoluteDenial = %v, want nil for an allowed send", err)
	}
}

// The refusal names a count and a bounded reason code, never a recipient: it
// becomes the detail of a client error, and a caller is owed what they can act
// on rather than somebody's consent history.
func TestTheStagingRefusalNamesNobody(t *testing.T) {
	set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		{
			Verdict:    commsauthz.VerdictDeny,
			ReasonCode: commsauthz.ReasonObjection,
			Recipient:  connector.Recipient{Email: "objector@example.test"},
		},
	}}
	err := refuseAbsoluteDenial(set)
	if err == nil {
		t.Fatal("want a refusal")
	}
	if got := err.Error(); strings.Contains(got, "objector@example.test") {
		t.Errorf("the refusal names the recipient: %q", got)
	}
}
