// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// What one run asks for, and the two rules that bound it.
//
// Automatic enrichment spends nothing: nobody weighed the purchase, so it
// takes only what the provider gives away. A human pressing a button may buy a
// priced category, but never one the connection does not carry — the admin's
// selection is a ceiling a rep cannot raise.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func pricedDescriptor() provider.Descriptor {
	return provider.Descriptor{
		Categories: []provider.Category{
			"linkedin_profile", "current_employment", "job_history",
			"professional_email", "mobile",
		},
		CostTable: map[provider.Category]map[provider.Pool]int{
			"linkedin_profile":   {},
			"current_employment": {},
			"job_history":        {},
			"professional_email": {"email": 1},
			"mobile":             {"mobile": 1},
		},
	}
}

func connectionBuying(categories ...string) admittedConnection {
	return admittedConnection{categories: categories}
}

func TestAnAutomaticRunBuysOnlyWhatCostsNothing(t *testing.T) {
	conn := connectionBuying("linkedin_profile", "job_history", "professional_email", "mobile")

	got, err := runCategories(pricedDescriptor(), conn, provider.QueueInput{
		Trigger: provider.TriggerAutomaticCreate,
	})
	if err != nil {
		t.Fatalf("narrowing the automatic run: %v", err)
	}

	// The connection permits two priced categories and the automatic lane
	// declines both: enrichment runs on every arrival precisely because it
	// costs nothing to let it.
	want := []provider.Category{"linkedin_profile", "job_history"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want only the free categories %v", got, want)
	}
	for i, category := range want {
		if got[i] != category {
			t.Errorf("position %d is %q, want %q", i, got[i], category)
		}
	}
}

func TestAHumanCanBuyAPricedCategoryForOnePerson(t *testing.T) {
	conn := connectionBuying("linkedin_profile", "professional_email")

	got, err := runCategories(pricedDescriptor(), conn, provider.QueueInput{
		Trigger:    provider.TriggerManual,
		Categories: []provider.Category{"professional_email"},
	})
	if err != nil {
		t.Fatalf("narrowing the manual run: %v", err)
	}

	// Exactly what was asked for, and nothing else — a button that said
	// "buy the work email" must not also spend on anything else it saw.
	if len(got) != 1 || got[0] != "professional_email" {
		t.Errorf("got %v, want only professional_email", got)
	}
}

func TestARunCannotBuyWhatTheConnectionDoesNotCarry(t *testing.T) {
	// An admin switched the mobile off for this installation.
	conn := connectionBuying("linkedin_profile", "professional_email")

	_, err := runCategories(pricedDescriptor(), conn, provider.QueueInput{
		Trigger:    provider.TriggerManual,
		Categories: []provider.Category{"mobile"},
	})

	// Refused, not quietly trimmed. Trimming would answer as though it had
	// complied while buying nothing, and the rep would never learn why.
	if !errors.Is(err, ErrCategoryNotPermitted) {
		t.Errorf("err = %v, want ErrCategoryNotPermitted", err)
	}
}

func TestAManualRunNamingNothingTakesTheWholeConnection(t *testing.T) {
	conn := connectionBuying("linkedin_profile", "professional_email")

	got, err := runCategories(pricedDescriptor(), conn, provider.QueueInput{
		Trigger: provider.TriggerManual,
	})
	if err != nil {
		t.Fatalf("narrowing the manual run: %v", err)
	}

	// The pre-existing behaviour, unchanged: a caller that names nothing gets
	// the connection's own selection, priced categories included.
	if len(got) != 2 {
		t.Errorf("got %v, want the connection's full selection", got)
	}
}

func TestAnAutomaticRunWithNothingFreeIsRefusedRatherThanDispatched(t *testing.T) {
	// An admin enabled automatic enrichment on a connection that buys only
	// priced categories.
	conn := connectionBuying("professional_email", "mobile")

	_, err := runCategories(pricedDescriptor(), conn, provider.QueueInput{
		Trigger: provider.TriggerAutomaticCreate,
	})

	// Dispatching it would send a request naming no categories, which a
	// provider answers with a refusal — and a refusal flips the connection's
	// status, breaking automatic ingest for every later contact over one
	// configuration choice.
	if !errors.Is(err, ErrNothingFreeToBuy) {
		t.Errorf("err = %v, want ErrNothingFreeToBuy", err)
	}
	// And the consumer swallows it: a policy that leaves nothing to buy is
	// working as configured, not failing.
	if !IsTriggerNotAdmitted(err) {
		t.Error("the refusal is not swallowed as a configuration state, so every arrival logs an error")
	}
}

func TestAFallbackCannotBeBoughtWithoutTheCategoryItFollows(t *testing.T) {
	desc := pricedDescriptor()
	desc.Cascades = []provider.Cascade{{
		Category: "personal_email",
		After:    "professional_email",
		Cost:     map[provider.Pool]int{"email": 2},
	}}
	desc.CostTable["personal_email"] = map[provider.Pool]int{}
	desc.Categories = append(desc.Categories, "personal_email")
	conn := connectionBuying("professional_email", "personal_email")

	_, err := runCategories(desc, conn, provider.QueueInput{
		Trigger:    provider.TriggerManual,
		Categories: []provider.Category{"personal_email"},
	})

	// The cascade never fires without its trigger, so the platform prices this
	// at nothing and reserves nothing — while the provider still charges for
	// whatever the email flag returns. A hold of zero against a real charge is
	// the one outcome this must not allow.
	if !errors.Is(err, ErrCategoryNotPermitted) {
		t.Errorf("err = %v, want the fallback refused without its trigger", err)
	}
}

// A category the provider only looks up when something else came back cannot
// be bought on its own.
//
// The defect this pins is the one a customer actually hit. Surfe's adapter
// always sends skipMobileEnrichmentIfNoEmailFound, so a request naming mobile
// alone makes the vendor hunt for an email nobody bought, fail, and skip the
// number. The run returns COMPLETED with no claims: no refusal, no error, a
// successful run — and the page reports the contact as found while the mobile
// field stays empty. Two of these were bought for one contact on consecutive
// days and neither could ever have produced a number.
func TestACategoryNeedingAnAnswerFirstCannotBeBoughtAlone(t *testing.T) {
	desc := pricedDescriptor()
	desc.RequiresAnswerTo = map[provider.Category]provider.Category{
		"mobile": "professional_email",
	}
	conn := connectionBuying("professional_email", "mobile")

	_, err := runCategories(desc, conn, provider.QueueInput{
		Trigger:    provider.TriggerManual,
		Categories: []provider.Category{"mobile"},
	})

	if !errors.Is(err, ErrCategoryNotPermitted) {
		t.Fatalf("err = %v, want mobile refused without the email it depends on", err)
	}
	// The message has to name BOTH halves: a reader told only that mobile was
	// refused cannot tell what to add.
	if msg := err.Error(); !strings.Contains(msg, "mobile") || !strings.Contains(msg, "professional_email") {
		t.Errorf("the refusal does not name both categories, so nobody can act on it: %v", err)
	}
}

// The same pair bought TOGETHER goes through. Without this case the rule above
// passes against a guard that refuses mobile outright, which would take the
// working purchase away with the broken one.
func TestTheDependentPairIsBoughtTogether(t *testing.T) {
	desc := pricedDescriptor()
	desc.RequiresAnswerTo = map[provider.Category]provider.Category{
		"mobile": "professional_email",
	}
	conn := connectionBuying("professional_email", "mobile")

	got, err := runCategories(desc, conn, provider.QueueInput{
		Trigger:    provider.TriggerManual,
		Categories: []provider.Category{"professional_email", "mobile"},
	})
	if err != nil {
		t.Fatalf("naming both categories was refused: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %v, want both categories — the run must ask for the pair it priced", got)
	}
}

// A provider declaring no prerequisite is unaffected. The rule reads the
// descriptor, so an adapter that never fills the map keeps today's behaviour
// rather than acquiring a constraint its vendor does not have.
func TestAProviderWithoutPrerequisitesBuysWhatItLikes(t *testing.T) {
	desc := pricedDescriptor()
	conn := connectionBuying("mobile")

	got, err := runCategories(desc, conn, provider.QueueInput{
		Trigger:    provider.TriggerManual,
		Categories: []provider.Category{"mobile"},
	})
	if err != nil {
		t.Fatalf("a provider declaring no prerequisite refused a lone category: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %v, want just the one category asked for", got)
	}
}

// A run naming NO categories takes the connection's selection — and that
// selection has to survive the same rules, because an admin ticking boxes on a
// settings card is not asked to know which the provider only issues alongside
// another.
//
// The defect: the guard above sat behind an early return for the empty request,
// so a connection selecting only the mobile queued exactly the broken
// mobile-only run through the API's own documented default.
func TestTheConnectionsOwnSelectionIsNarrowedToWhatCanBeAsked(t *testing.T) {
	desc := pricedDescriptor()
	desc.RequiresAnswerTo = map[provider.Category]provider.Category{
		"mobile": "professional_email",
	}
	// The admin ticked the mobile and the free ones, and left the work email
	// off — a configuration nothing forbids.
	conn := connectionBuying("linkedin_profile", "mobile")

	got, err := runCategories(desc, conn, provider.QueueInput{Trigger: provider.TriggerManual})
	if err != nil {
		t.Fatalf("a plain lookup was refused over a category nobody typed: %v", err)
	}
	for _, c := range got {
		if c == "mobile" {
			t.Errorf("got %v — the mobile is still asked for without the email it needs, which is the "+
				"request that comes back completed and empty", got)
		}
	}
	if len(got) != 1 || got[0] != "linkedin_profile" {
		t.Errorf("got %v, want just linkedin_profile: what the provider CAN answer must survive", got)
	}
}

// Dropping must not empty the request. A connection whose every selection
// depends on something it does not select is refused, because a request naming
// no categories is one the provider answers with a refusal — and that flips the
// connection's status, breaking every later lookup over one configuration.
func TestAConnectionWhoseWholeSelectionIsUnaskableIsRefused(t *testing.T) {
	desc := pricedDescriptor()
	desc.RequiresAnswerTo = map[provider.Category]provider.Category{
		"mobile": "professional_email",
	}
	conn := connectionBuying("mobile")

	_, err := runCategories(desc, conn, provider.QueueInput{Trigger: provider.TriggerManual})
	if !errors.Is(err, ErrCategoryNotPermitted) {
		t.Fatalf("err = %v, want a refusal: dispatching the empty remainder sends a request naming no "+
			"categories, and the refusal that comes back marks the connection broken", err)
	}
}

// A selection with no dependencies at all is returned untouched, so the
// narrowing above cannot quietly shrink an ordinary connection.
func TestAnUnencumberedSelectionIsReturnedWhole(t *testing.T) {
	desc := pricedDescriptor()
	conn := connectionBuying("linkedin_profile", "professional_email", "mobile")

	got, err := runCategories(desc, conn, provider.QueueInput{Trigger: provider.TriggerManual})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("got %v, want all three — a provider declaring no prerequisites loses nothing", got)
	}
}
