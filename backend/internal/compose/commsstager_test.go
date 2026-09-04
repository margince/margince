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
		err := refuseAtStaging(set)
		if !errors.Is(err, apperrors.ErrConsentNotGranted) {
			t.Errorf("%s staged anyway: err = %v", reason, err)
		}
	}
}

// And the converse, or the test above would pass with every send refused: an
// ordinary disagreement under a category still being OBSERVED is recorded and
// let through. The engine's answer carries no authority there, the old gate
// decides, and blocking on a difference nobody has reviewed would refuse
// legitimate mail.
func TestStagingLetsAnObservedDisagreementThrough(t *testing.T) {
	set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		{
			Verdict: commsauthz.VerdictReview, ReasonCode: commsauthz.ReasonNoEvidence,
			Mode: commsauthz.ModeObserve,
		},
	}}
	if err := refuseAtStaging(set); err != nil {
		t.Errorf("refuseAtStaging = %v, want nil for a category still being observed", err)
	}
}

// AN ORDINARY REFUSAL UNDER AN ENFORCED CATEGORY IS REFUSED AT STAGING, and
// that is the arm the flip added. The transmit phase would refuse the same send
// anyway, so staging it buys nothing: the rep learns hours later from a parked
// row, and the activity commits first, minting outbound correspondence evidence
// for a message that never went.
//
// Mutation: drop the enforced arm from refuseAtStaging and this fails.
func TestStagingRefusesAnOrdinaryDenialUnderEnforce(t *testing.T) {
	for _, verdict := range []commsauthz.Verdict{commsauthz.VerdictDeny, commsauthz.VerdictReview} {
		set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
			{Verdict: verdict, ReasonCode: commsauthz.ReasonNoEvidence, Mode: commsauthz.ModeEnforce},
		}}
		if err := refuseAtStaging(set); !errors.Is(err, apperrors.ErrConsentNotGranted) {
			t.Errorf("an enforced %s staged anyway: err = %v", verdict, err)
		}
	}
}

// ONE ENFORCED REFUSAL REFUSES THE MESSAGE, even beside a recipient whose own
// category is still observed. Whole-message refusal is the shape both
// authorities already had, and a per-recipient split here would send a message
// to some of the people it names and quietly drop the rest.
func TestOneEnforcedRefusalRefusesTheWholeMessage(t *testing.T) {
	set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		{
			Verdict: commsauthz.VerdictAllow, ReasonCode: commsauthz.ReasonAllowed,
			Mode: commsauthz.ModeEnforce,
		},
		{
			Verdict: commsauthz.VerdictReview, ReasonCode: commsauthz.ReasonNoEvidence,
			Mode: commsauthz.ModeObserve,
		},
		{
			Verdict: commsauthz.VerdictDeny, ReasonCode: commsauthz.ReasonNoEvidence,
			Mode: commsauthz.ModeEnforce,
		},
	}}
	if err := refuseAtStaging(set); !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Errorf("a message with one enforced refusal staged anyway: err = %v", err)
	}
}

// A message every recipient may receive is not refused for any reason.
func TestStagingAllowsAnOrdinarySend(t *testing.T) {
	set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		{Verdict: commsauthz.VerdictAllow, ReasonCode: commsauthz.ReasonAllowed},
	}}
	if err := refuseAtStaging(set); err != nil {
		t.Errorf("refuseAtStaging = %v, want nil for an allowed send", err)
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
	err := refuseAtStaging(set)
	if err == nil {
		t.Fatal("want a refusal")
	}
	if got := err.Error(); strings.Contains(got, "objector@example.test") {
		t.Errorf("the refusal names the recipient: %q", got)
	}
}
