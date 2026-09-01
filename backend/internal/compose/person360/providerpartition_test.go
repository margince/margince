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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	profiles, err := svc.profilesFor(namesToShow(connected, nil, claims), connected, nil, claims, lookupableSubject())
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
	profiles, err := svc.profilesFor(namesToShow(connected, nil, nil), connected, nil, nil, lookupableSubject())
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
	profiles, err := svc.profilesFor(namesToShow(connected, nil, nil), connected, nil, nil, lookupableSubject())
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
	profiles, err := svc.profilesFor(namesToShow(connected, nil, claims), connected, nil, claims, lookupableSubject())
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
			profiles, err := svc.profilesFor(namesToShow(statuses, nil, claims), statuses, nil, claims, lookupableSubject())
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

// A run that answered ONE category out of six is the case this reports. It
// rendered as a plain success with five blank fields, and the reader could not
// tell an empty purchase from a full one — the defect that prompted this.
func TestASectionNamesTheCategoriesTheProviderHadNothingFor(t *testing.T) {
	runID := ids.NewV7()
	desc := provider.Descriptor{
		Categories: []provider.Category{"professional_email", "mobile", "current_employment"},
		Answers: map[provider.Category][]provider.ClaimKey{
			"professional_email": {provider.ClaimProfessionalEmails},
			"mobile":             {provider.ClaimMobilePhones},
			"current_employment": {provider.ClaimCurrentEmployment},
		},
	}
	// The provider answered the employment and nothing else.
	claims := []storedClaim{{
		key:   string(provider.ClaimCurrentEmployment),
		runID: runID,
	}}
	requested := []string{"professional_email", "mobile", "current_employment"}

	got := categoriesWithoutAnswer(desc, requested, deliveredKeys(runID, claims))

	want := []string{"mobile", "professional_email"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want the two categories that came back empty (%v)", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("position %d is %q, want %q", i, got[i], name)
		}
	}
}

func TestACategoryAnsweredByAnOlderRunIsStillReportedSilentForTheLatest(t *testing.T) {
	older, latest := ids.NewV7(), ids.NewV7()
	desc := provider.Descriptor{
		Categories: []provider.Category{"mobile"},
		Answers: map[provider.Category][]provider.ClaimKey{
			"mobile": {provider.ClaimMobilePhones},
		},
	}
	// An earlier run bought a number; the latest run asked again and got none.
	claims := []storedClaim{{key: string(provider.ClaimMobilePhones), runID: older}}

	got := categoriesWithoutAnswer(desc, []string{"mobile"}, deliveredKeys(latest, claims))

	// The reader is deciding whether the run they just paid for was worth it.
	// Reading the union would call it answered on the strength of a purchase
	// made before it, and hide that this run returned nothing.
	if len(got) != 1 || got[0] != "mobile" {
		t.Errorf("got %v, want mobile reported silent for the latest run", got)
	}
}

func TestACategoryTheAdapterNeverMappedIsNotAccusedOfSilence(t *testing.T) {
	runID := ids.NewV7()
	// The adapter declared no correspondence for this category.
	desc := provider.Descriptor{Categories: []provider.Category{"exotic"}}

	got := categoriesWithoutAnswer(desc, []string{"exotic"}, deliveredKeys(runID, nil))

	// Silence about the mapping is not evidence the provider withheld
	// anything, and reporting it would blame a vendor for the adapter's gap.
	if len(got) != 0 {
		t.Errorf("got %v, want nothing reported for an undeclared category", got)
	}
}

func TestAFallbackThatNeverFiredIsNotReportedAsUnanswered(t *testing.T) {
	runID := ids.NewV7()
	// Surfe's shape: the personal-email pass runs only when the professional
	// one comes back empty.
	desc := provider.Descriptor{
		Categories: []provider.Category{"professional_email", "personal_email"},
		Answers: map[provider.Category][]provider.ClaimKey{
			"professional_email": {provider.ClaimProfessionalEmails},
			"personal_email":     {provider.ClaimPersonalEmails},
		},
		Cascades: []provider.Cascade{{
			Category: "personal_email",
			After:    "professional_email",
		}},
	}
	// The professional pass answered, so the fallback was never issued.
	claims := []storedClaim{{
		key:   string(provider.ClaimProfessionalEmails),
		runID: runID,
	}}
	requested := []string{"professional_email", "personal_email"}

	got := categoriesWithoutAnswer(desc, requested, deliveredKeys(runID, claims))

	// Reporting the fallback would claim the provider was asked and had
	// nothing, on a line whose whole job is saying what the money bought.
	if len(got) != 0 {
		t.Errorf("got %v, want nothing — the fallback was never sent", got)
	}
}

func TestAFallbackThatDidFireIsReportedWhenItFoundNothing(t *testing.T) {
	runID := ids.NewV7()
	desc := provider.Descriptor{
		Categories: []provider.Category{"professional_email", "personal_email"},
		Answers: map[provider.Category][]provider.ClaimKey{
			"professional_email": {provider.ClaimProfessionalEmails},
			"personal_email":     {provider.ClaimPersonalEmails},
		},
		Cascades: []provider.Cascade{{
			Category: "personal_email",
			After:    "professional_email",
		}},
	}
	requested := []string{"professional_email", "personal_email"}

	// Neither answered: the professional pass came back empty, which is
	// exactly what issues the fallback, and it found nothing either.
	got := categoriesWithoutAnswer(desc, requested, deliveredKeys(runID, nil))

	want := []string{"personal_email", "professional_email"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want both reported (%v)", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("position %d is %q, want %q", i, got[i], name)
		}
	}
}

func TestASkippedPrerequisiteMeansTheCategoryWasNeverAsked(t *testing.T) {
	runID := ids.NewV7()
	// Surfe sends SkipMobileEnrichmentIfNoEmailFound: a subject it cannot
	// place by email is never asked for a number.
	desc := provider.Descriptor{
		Categories: []provider.Category{"professional_email", "mobile"},
		Answers: map[provider.Category][]provider.ClaimKey{
			"professional_email": {provider.ClaimProfessionalEmails},
			"mobile":             {provider.ClaimMobilePhones},
		},
		RequiresAnswerTo: map[provider.Category]provider.Category{
			"mobile": "professional_email",
		},
	}

	// No email found, so no mobile lookup was ever issued.
	got := categoriesWithoutAnswer(desc, []string{"professional_email", "mobile"},
		deliveredKeys(runID, nil))

	// The email is a real silence; the mobile is a question nobody asked.
	if len(got) != 1 || got[0] != "professional_email" {
		t.Errorf("got %v, want only professional_email — no mobile lookup was sent", got)
	}
}

func TestOnlyACompletedRunWithItsClaimsWrittenSpeaksForTheProvider(t *testing.T) {
	for _, tc := range []struct {
		name    string
		run     providerRunRow
		answers bool
	}{
		{"completed", providerRunRow{state: "completed"}, true},
		{"still queued", providerRunRow{state: "queued"}, false},
		{"in flight", providerRunRow{state: "in_progress"}, false},
		{"skipped without calling", providerRunRow{state: "skipped"}, false},
		{"failed", providerRunRow{state: "failed"}, false},
		{"outcome never learned", providerRunRow{state: "submission_unknown"}, false},
		// The provider DID answer and the hand-off dropped it. Reporting this
		// as provider silence blames the vendor for our own defect.
		{"claims never written", providerRunRow{state: "completed", claimsUnwritten: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if answerable(tc.run) != tc.answers {
				t.Errorf("answerable(%s) = %v, want %v", tc.name, !tc.answers, tc.answers)
			}
		})
	}
}

func TestTheAskedSetExcludesWhatWasNeverSent(t *testing.T) {
	runID := ids.NewV7()
	desc := provider.Descriptor{
		Categories: []provider.Category{"professional_email", "mobile", "personal_email"},
		Answers: map[provider.Category][]provider.ClaimKey{
			"professional_email": {provider.ClaimProfessionalEmails},
			"mobile":             {provider.ClaimMobilePhones},
			"personal_email":     {provider.ClaimPersonalEmails},
		},
		Cascades: []provider.Cascade{{
			Category: "personal_email",
			After:    "professional_email",
		}},
		RequiresAnswerTo: map[provider.Category]provider.Category{
			"mobile": "professional_email",
		},
	}
	// The professional pass answered: the fallback never fired, and the mobile
	// lookup was sent because its prerequisite was satisfied.
	claims := []storedClaim{{key: string(provider.ClaimProfessionalEmails), runID: runID}}
	requested := []string{"professional_email", "mobile", "personal_email"}

	got := asked(desc, requested, deliveredKeys(runID, claims))

	// Two questions reached the provider, not three. Counting the unfired
	// fallback would report a lookup nobody made, and the receipt divides by
	// this number.
	want := []string{"mobile", "professional_email"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v — the fallback never fired", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("position %d is %q, want %q", i, got[i], name)
		}
	}
}

// lookupableSubject is a contact a provider CAN match on, which is what these
// partition cases are about: they ask which provider's values land in which
// section, and a subject nothing can look up would answer every one of them
// with "nothing to look them up by" before the partition was reached.
func lookupableSubject() provider.PersonIdentifiers {
	return provider.PersonIdentifiers{
		FirstName:   "Anna",
		LastName:    "Muster",
		CompanyName: "Example GmbH",
	}
}
