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
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	if len(runs) == 0 {
		// Nobody has asked, and nothing on this record could be asked WITH.
		// The reader's next step is to add a profile link or an employer, not
		// to press a button admission will decline.
		if !lookupable {
			return crmcontracts.PersonProviderProfileStateNothingToLookUp
		}
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
	// A run that FAILED on a record nothing can match on is a gravestone, not
	// news about the vendor.
	//
	// Before admission checked for identifiers, such a contact was sent
	// anyway; the provider rejected the request and the platform stored that
	// as a provider fault. Left to speak, those runs tell the reader the
	// vendor is broken and a retry will fix it, and neither is true — a retry
	// is declined before it reaches anybody.
	//
	// ONLY the failure states, and only here at the end. A completed run whose
	// claims never landed means somebody was CHARGED and has nothing to show
	// for it, and a submission whose outcome was never learned may have been
	// charged too. Those are material facts about money that outrank a missing
	// employer, so they are answered above and reach this line only if the run
	// is neither.
	if !lookupable && staleFault[latest.state] {
		return crmcontracts.PersonProviderProfileStateNothingToLookUp
	}
	if mapped, ok := providerRunStates[latest.state]; ok {
		return mapped
	}
	return crmcontracts.PersonProviderProfileStateProviderError
}

// staleFault names the run outcomes that say nothing once the record itself
// cannot be looked up. A failed or cancelled run bought nothing and charged
// nothing, so replacing it with the record's own fact costs the reader no
// information — unlike a completed or unknown one, which may have cost money.
var staleFault = map[string]bool{
	string(provider.RunFailed):    true,
	string(provider.RunCancelled): true,
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

// matchIdentifiers resolves what a provider could match this contact on, under
// the grants the caller actually holds.
//
// The full answer includes the employer, which is a relationship read — and
// this section is entered on the `person` grant alone. So the edge is read only
// where the caller may read edges, exactly as every other section here does,
// and a caller without that grant gets a name-only answer.
//
// Erring toward FINDABLE: a name-only answer can only make a contact look less
// matchable than they are, and the page's own fallback for "findable" is the
// state it showed before this rule existed. The opposite — telling somebody a
// record cannot be looked up because we were not allowed to see the field that
// would have proved otherwise — invents a fact out of a permission.
func (s *Service) matchIdentifiers(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (provider.PersonIdentifiers, error) {
	if err := requireRead(ctx, "relationship"); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return people.SubjectNameOnly(ctx, tx, personID.String())
		}
		return provider.PersonIdentifiers{}, err
	}
	return people.SubjectIdentifiers(ctx, tx, personID.String())
}
