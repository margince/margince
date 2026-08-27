// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// One section per provider, and no value crossing between them.
//
// The page exists to tell a reader who was paid for which value. A fold that
// mixed two vendors' answers into one list would put a number nobody bought
// from Surfe under Surfe's heading, and the reader spends money on the
// strength of that attribution.
//
// Before the split this was live: every claim folded into ONE profile, so the
// single-valued fields (location, current employment, LinkedIn) were
// last-write-wins across vendors — whichever retrieved most recently silently
// won, under whatever name the newest RUN happened to carry.

import (
	"encoding/json"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func locationClaim(t *testing.T, providerName, city string, at time.Time) storedClaim {
	t.Helper()
	raw, err := json.Marshal(city)
	if err != nil {
		t.Fatalf("encoding the fixture claim: %v", err)
	}
	return storedClaim{
		key:         string(provider.ClaimLocation),
		value:       raw,
		provider:    providerName,
		retrievedAt: at,
	}
}

func TestEachProviderSectionCarriesOnlyItsOwnPurchases(t *testing.T) {
	earlier := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	later := earlier.Add(24 * time.Hour)
	// Two vendors answered the same question differently, and the second
	// answered later. Folded together, the newer one wins outright and the
	// older vendor's paid-for answer disappears from the page.
	claims := []storedClaim{
		locationClaim(t, "surfe", "Munich, Germany", earlier),
		locationClaim(t, "otherco", "Berlin, Germany", later),
	}
	connected := map[string]string{"surfe": "connected", "otherco": "connected"}

	svc := &Service{}
	profiles, err := svc.profilesFor(namesToShow(connected, nil, claims), connected, nil, claims)
	if err != nil {
		t.Fatalf("folding the sections: %v", err)
	}

	if len(profiles) != 2 {
		t.Fatalf("got %d sections, want one per connected provider", len(profiles))
	}
	byName := map[string]string{}
	for _, p := range profiles {
		if p.Location == nil {
			t.Fatalf("section %q carries no location, so its own purchase went missing", p.Provider)
		}
		byName[string(p.Provider)] = *p.Location
	}
	if byName["surfe"] != "Munich, Germany" {
		t.Errorf("surfe's section shows %q, want the value bought FROM surfe", byName["surfe"])
	}
	if byName["otherco"] != "Berlin, Germany" {
		t.Errorf("otherco's section shows %q, want the value bought FROM otherco", byName["otherco"])
	}
}

func TestSectionsKeepAStableOrderBetweenReads(t *testing.T) {
	connected := map[string]string{"zeta": "connected", "alpha": "connected", "surfe": "connected"}

	svc := &Service{}
	profiles, err := svc.profilesFor(namesToShow(connected, nil, nil), connected, nil, nil)
	if err != nil {
		t.Fatalf("folding the sections: %v", err)
	}

	// Sorted by name: a map iteration order would reshuffle the sections
	// between two reads of the same page, moving a button under the reader's
	// cursor between the moment they aimed and the moment they clicked.
	want := []string{"alpha", "surfe", "zeta"}
	if len(profiles) != len(want) {
		t.Fatalf("got %d sections, want %d", len(profiles), len(want))
	}
	for i, name := range want {
		if string(profiles[i].Provider) != name {
			t.Errorf("section %d is %q, want %q", i, profiles[i].Provider, name)
		}
	}
}

func TestAProviderNobodyRunsYetStillGetsItsSection(t *testing.T) {
	connected := map[string]string{"surfe": "connected"}

	svc := &Service{}
	profiles, err := svc.profilesFor(namesToShow(connected, nil, nil), connected, nil, nil)
	if err != nil {
		t.Fatalf("folding the sections: %v", err)
	}

	// never_run, present rather than absent: the reader reaches the lookup
	// through this section, so a page that omitted it would leave them with a
	// connected provider and no way to ask it anything.
	if len(profiles) != 1 {
		t.Fatalf("got %d sections, want the connected provider's", len(profiles))
	}
	if profiles[0].State != "never_run" {
		t.Errorf("state is %q, want never_run", profiles[0].State)
	}
}

func TestAPurchaseSurvivesItsProviderBeingDisconnected(t *testing.T) {
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	claims := []storedClaim{locationClaim(t, "otherco", "Berlin, Germany", at)}
	// Nobody is connected to otherco any more, but it sold us something.
	connected := map[string]string{"surfe": "connected"}

	svc := &Service{}
	profiles, err := svc.profilesFor(namesToShow(connected, nil, claims), connected, nil, claims)
	if err != nil {
		t.Fatalf("folding the sections: %v", err)
	}

	// Disconnecting stops new egress; it does not delete what was bought. A
	// page that dropped the section would hide a purchase the customer paid
	// for, which is the one thing worse than showing it without a button.
	var otherco *string
	for _, p := range profiles {
		if string(p.Provider) == "otherco" {
			otherco = p.Location
		}
	}
	if otherco == nil {
		t.Fatal("the disconnected provider's purchase vanished from the page")
	}
	if *otherco != "Berlin, Germany" {
		t.Errorf("the retained purchase reads %q, want what was bought", *otherco)
	}
}

func TestAnImpairedConnectionSaysWhatIsWrongRatherThanReadingAsStale(t *testing.T) {
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	claims := []storedClaim{locationClaim(t, "surfe", "Munich, Germany", at)}

	for _, tc := range []struct {
		status string
		want   crmcontracts.PersonProviderProfileState
	}{
		{"invalid_credentials", crmcontracts.PersonProviderProfileStateInvalidCredentials},
		{"insufficient_credits", crmcontracts.PersonProviderProfileStateInsufficientCredits},
		{"rate_limited", crmcontracts.PersonProviderProfileStateRateLimited},
		{"provider_error", crmcontracts.PersonProviderProfileStateProviderError},
	} {
		t.Run(tc.status, func(t *testing.T) {
			statuses := map[string]string{"surfe": tc.status}

			svc := &Service{}
			profiles, err := svc.profilesFor(namesToShow(statuses, nil, claims), statuses, nil, claims)
			if err != nil {
				t.Fatalf("folding the sections: %v", err)
			}

			// The connection's own condition, not `stale`. Stale says nobody is
			// connected any more, which sends an operator looking at the contact
			// for a problem that is in the settings — and the four conditions
			// here are the ones they can actually fix.
			if len(profiles) != 1 {
				t.Fatalf("got %d sections, want one", len(profiles))
			}
			if profiles[0].State != tc.want {
				t.Errorf("a %s connection reads as %q, want %q", tc.status, profiles[0].State, tc.want)
			}
		})
	}
}
