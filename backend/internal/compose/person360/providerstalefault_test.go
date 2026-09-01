// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// What a failed run means depends on whether the contact could ever have been
// found.
//
// Before admission checked for identifiers, a contact carrying neither a
// profile link nor an employer was sent to the provider anyway. The vendor
// rejected the request and the platform stored that as a provider fault. Those
// runs are still on record: in one installation, 205 contacts had one as their
// newest run, and every one of their pages read "The last call to the provider
// failed. Automatic lookups are paused; a free check that gets through resumes
// them" — over a connection that was `connected` and working.
//
// Both halves of that sentence were false, and the reader could act on
// neither. What is true is that the record carries nothing to look them up by.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func failedRun() []providerRunRow {
	return []providerRunRow{{
		state:    string(provider.RunFailed),
		safeCode: "provider_error",
	}}
}

func TestAFailedRunOnAnUnlookupableContactIsNotAProviderFault(t *testing.T) {
	t.Parallel()

	got := resolveProviderState(failedRun(), "connected", false)
	if got != crmcontracts.PersonProviderProfileStateNothingToLookUp {
		t.Errorf("state = %q, want nothing_to_look_up.\n\n"+
			"A contact with no profile link and no employer cannot be looked up, so a past refusal is not "+
			"news about the provider — and the provider_error copy tells the reader that automatic lookups "+
			"are paused and a retry resumes them, when the connection is healthy and a retry can only be "+
			"skipped again.", got)
	}
}

// The same run on a contact the provider CAN match on still reads as a
// provider fault. Without this case the rule above passes against one that
// swallows every failure, which would hide the outage it exists to report.
func TestAFailedRunOnAFindableContactStillReadsAsAProviderFault(t *testing.T) {
	t.Parallel()

	got := resolveProviderState(failedRun(), "connected", true)
	if got != crmcontracts.PersonProviderProfileStateProviderError {
		t.Errorf("state = %q, want provider_error: this contact carries something to match on, so a "+
			"refusal really is the vendor's and the reader should see it", got)
	}
}

// A contact nothing can look up says so even where no run exists at all, which
// is the case every newly captured calendar contact is in. never_run would
// invite a press of a button that can only be skipped.
func TestAnUnlookupableContactSaysSoBeforeAnybodyHasAsked(t *testing.T) {
	t.Parallel()

	got := resolveProviderState(nil, "connected", false)
	if got != crmcontracts.PersonProviderProfileStateNothingToLookUp {
		t.Errorf("state = %q, want nothing_to_look_up: inviting a lookup that admission will decline "+
			"teaches a reader that the button does nothing", got)
	}
}

// The connection's own condition still outranks the record's. A refused key is
// the installation's problem and the only thing anybody can act on, whatever
// this one contact carries.
func TestTheConnectionsOwnFaultOutranksAnUnlookupableRecord(t *testing.T) {
	t.Parallel()

	for status, want := range map[string]crmcontracts.PersonProviderProfileState{
		"invalid_credentials":  crmcontracts.PersonProviderProfileStateInvalidCredentials,
		"insufficient_credits": crmcontracts.PersonProviderProfileStateInsufficientCredits,
		"rate_limited":         crmcontracts.PersonProviderProfileStateRateLimited,
	} {
		if got := resolveProviderState(failedRun(), status, false); got != want {
			t.Errorf("status %q with an unlookupable contact = %q, want %q: rotating a refused key is "+
				"what unblocks every contact, and burying that behind one record's missing employer "+
				"leaves an admin with nothing to fix", status, got, want)
		}
	}
}

// A disconnected provider still reports what was PAID for. Purchases outlive
// the connection, and telling a reader there is nothing to look up while
// showing them a bought mobile number is the confusion `stale` exists to stop.
func TestADisconnectedProviderStillSaysStaleOverAnUnlookupableRecord(t *testing.T) {
	t.Parallel()

	got := resolveProviderState(failedRun(), "disconnected", false)
	if got != crmcontracts.PersonProviderProfileStateStale {
		t.Errorf("state = %q, want stale: this person has runs on record, and what was bought is still "+
			"on the page below", got)
	}
}
