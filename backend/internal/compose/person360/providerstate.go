// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// Which sentence the provider section shows, and why that one.
//
// Split from the assembly beside it because they answer different questions:
// that file gathers what a provider was asked and what it sold us, and this one
// decides what to TELL the reader about it. The vocabularies are separate on
// purpose — a run state is what the platform is doing, and a profile state is
// what somebody reads.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// impairedStates maps a connection that EXISTS but cannot buy onto the sentence
// the page shows. Each names the installation's own condition, because that is
// where the fix is — none of them is a fact about the contact.
var impairedStates = map[string]crmcontracts.PersonProviderProfileState{
	"invalid_credentials":  crmcontracts.PersonProviderProfileStateInvalidCredentials,
	"insufficient_credits": crmcontracts.PersonProviderProfileStateInsufficientCredits,
	"rate_limited":         crmcontracts.PersonProviderProfileStateRateLimited,
	"provider_error":       crmcontracts.PersonProviderProfileStateProviderError,
}

// lookupable asks one provider's own rules whether this record carries
// anything it could match on. A provider this build does not carry answers
// true: the page must not report a record as unfindable on the authority of a
// descriptor it cannot read.
func (s *Service) lookupable(name string, idents provider.PersonIdentifiers) bool {
	if s.providers == nil {
		return true
	}
	desc, err := s.providers.Descriptor(name)
	if err != nil {
		return true
	}
	return idents.Matchable(desc.MatchRules)
}

// resolveProviderState answers the ONE sentence the page shows about this
// person's enrichment. The "nothing here" states are separate facts and must
// never collapse into one: nobody connected a provider, this person is not
// eligible for one, this record carries nothing to look them up by, and nobody
// has asked yet are different answers to "why is this empty", and only the
// reader can act on the right one.
func resolveProviderState(runs []providerRunRow, status string, lookupable bool) crmcontracts.PersonProviderProfileState {
	// A connection that exists but cannot buy says WHY, before anything about
	// this person's runs. The condition is the installation's — a refused key,
	// an exhausted wallet — and it outranks the run history because it is what
	// the next lookup will hit, and the only thing anybody can act on.
	if impaired, ok := impairedStates[status]; ok {
		return impaired
	}
	if status != "connected" {
		// Disconnected, and the data this installation already PAID for is
		// still on the page below — disconnecting stops new egress, it does
		// not delete what was bought. So the honest word is stale, not
		// not_connected: telling the reader the platform knows nothing while
		// showing them a purchased mobile number is the one thing this state
		// exists to prevent. not_connected is for a page with nothing on it.
		if len(runs) > 0 {
			return crmcontracts.PersonProviderProfileStateStale
		}
		return crmcontracts.PersonProviderProfileStateNotConnected
	}
	// Nothing on this record for the provider to match on, so nothing that
	// happened to a past run is the fact the reader needs. This outranks the
	// run history for the same reason the connection's own condition does: it
	// is what the NEXT lookup will hit, and unlike a run it is something the
	// reader can act on — add a profile link or an employer.
	//
	// It is also the only honest reading of the runs already on record. Before
	// admission checked this, an unlookupable contact was sent anyway and the
	// vendor rejected the request; the platform stored that as a provider
	// fault. Those runs are still there, and left to speak they tell every one
	// of those contacts that the vendor is broken and a retry will fix it.
	// Neither half is true.
	if !lookupable {
		return crmcontracts.PersonProviderProfileStateNothingToLookUp
	}
	if len(runs) == 0 {
		return crmcontracts.PersonProviderProfileStateNeverRun
	}
	latest := runs[0]
	if latest.state == string(provider.RunCompleted) && latest.claimsUnwritten {
		// Paid, and the values never reached the record. Its own state
		// because it is neither a success nor a failure: the customer was
		// charged and has nothing to show for it, which is a thing somebody
		// needs to see rather than a completed run with empty fields.
		return crmcontracts.PersonProviderProfileStateCompletedClaimsUnwritten
	}
	if latest.state == string(provider.RunSkipped) {
		// A skip is a decision, and its REASON is what the reader needs. An
		// exhausted budget is a fact about the installation's wallet; telling
		// somebody "this person is not eligible" instead would send them
		// looking at the contact for a problem that is not there.
		return skipStates[latest.skipReason]
	}
	if mapped, ok := providerRunStates[latest.state]; ok {
		return mapped
	}
	return crmcontracts.PersonProviderProfileStateProviderError
}

// skipStates says WHY nothing was bought, in the page's vocabulary. The zero
// value is not_eligible, which is the honest default for the two reasons that
// really are about the subject; everything else names the installation's own
// condition instead.
var skipStates = map[string]crmcontracts.PersonProviderProfileState{
	"":                            crmcontracts.PersonProviderProfileStateNotEligible,
	"not_eligible":                crmcontracts.PersonProviderProfileStateNotEligible,
	"duplicate_subject_candidate": crmcontracts.PersonProviderProfileStateNotEligible,
	// A consent decision, not an eligibility one. It reads as not_eligible
	// today because the contract has no suppressed state; the difference is
	// worth a spec change rather than a wrong word invented here.
	"suppressed": crmcontracts.PersonProviderProfileStateNotEligible,
	// Its own state, not a kind of not_eligible: nothing forbids this
	// purchase. The record carries no profile link and no company, so the
	// provider has nothing to match on — and the reader's next step is to add
	// one of those, which is the next step for nothing else in this map.
	"no_identifiers":   crmcontracts.PersonProviderProfileStateNothingToLookUp,
	"budget_exhausted": crmcontracts.PersonProviderProfileStateInsufficientCredits,
	"low_balance":      crmcontracts.PersonProviderProfileStateInsufficientCredits,
	"rate_limited":     crmcontracts.PersonProviderProfileStateRateLimited,
	// Nothing was bought because we already hold an answer inside the refresh
	// window — which is a completed enrichment, not a refusal.
	"already_fresh": crmcontracts.PersonProviderProfileStateCompleted,
}

// providerRunStates maps the run machine onto the page's vocabulary. The two
// are deliberately separate: a run state is what the platform is doing, and
// this is what the reader is told.
var providerRunStates = map[string]crmcontracts.PersonProviderProfileState{
	string(provider.RunQueued):            crmcontracts.PersonProviderProfileStateQueued,
	string(provider.RunSubmitting):        crmcontracts.PersonProviderProfileStateInProgress,
	string(provider.RunInProgress):        crmcontracts.PersonProviderProfileStateInProgress,
	string(provider.RunCompleted):         crmcontracts.PersonProviderProfileStateCompleted,
	string(provider.RunNoMatch):           crmcontracts.PersonProviderProfileStateNoMatch,
	string(provider.RunSubmissionUnknown): crmcontracts.PersonProviderProfileStateSubmissionUnknown,
	string(provider.RunCancelled):         crmcontracts.PersonProviderProfileStateNeverRun,
}
