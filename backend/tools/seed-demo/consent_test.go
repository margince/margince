// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// TestGrantWordingPrefersTheDatasetsOwnSentence is the point of the field. A
// demo exists to be read, and the consent surfaces read back what the subject
// was shown — so a dataset that has authored real form copy must see that copy
// on the proof row, not a sentence the seeder made up around it.
func TestGrantWordingPrefersTheDatasetsOwnSentence(t *testing.T) {
	want := "I agree to receive product news from Gradion by email."
	got := grantWording(demoConsent{Wording: want}, "Marketing email")
	if got != want {
		t.Errorf("grantWording = %q, want the dataset's own sentence %q", got, want)
	}
}

// TestGrantWordingFallbackNamesItselfAsSeeded is the constraint #4583 argues
// for. That change DELETED the old 'recorded via API' placeholder because a row
// carrying it "reads like evidence and holds none" — so the fallback here must
// not be plausible consent copy. It has to say, on the row itself, that no
// subject was shown anything, or this fix reintroduces exactly what upstream
// removed.
func TestGrantWordingFallbackNamesItselfAsSeeded(t *testing.T) {
	got := grantWording(demoConsent{Purpose: "marketing_email"}, "Marketing email")
	if got == "" {
		t.Fatal("grantWording returned empty, which the API refuses for a grant")
	}
	for _, forbidden := range []string{"I agree", "I consent", "Yes, please"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("fallback %q reads as real consent copy (%q); a seeded row must not claim a screen existed", got, forbidden)
		}
	}
	if !strings.Contains(got, "Marketing email") {
		t.Errorf("fallback %q does not name the purpose it stands in for", got)
	}
	if !strings.Contains(got, "seeded") {
		t.Errorf("fallback %q does not say it is seeded demo data", got)
	}
}

// TestGrantWordingStaysUnderTheContractsBound guards the 2000-rune ceiling the
// API enforces. A label is dataset-supplied and nothing upstream bounds it, so
// a long one must not push the fallback over a limit that would 422 the whole
// seeding run — which is the failure this fix exists to remove.
func TestGrantWordingStaysUnderTheContractsBound(t *testing.T) {
	long := ""
	for range 3000 {
		long += "x"
	}
	if n := len([]rune(grantWording(demoConsent{}, long))); n > 2000 {
		t.Errorf("fallback is %d runes, over the contract's 2000", n)
	}
}

// TestConsentBodyCarriesWordingOnlyForAGrant pins the asymmetry #4583 built
// into the writer. A grant must carry wording or the API refuses it; a
// withdrawal must not, because wordingFor drops it anyway and a withdrawal row
// that arrived holding the sentence from a GRANT is the exact false claim that
// change exists to make impossible.
func TestConsentBodyCarriesWordingOnlyForAGrant(t *testing.T) {
	purpose := consentPurpose{id: "11111111-1111-1111-1111-111111111111", label: "Marketing email"}

	granted := consentBody(purpose, demoConsent{State: "granted", Wording: "I agree to receive product news."})
	if got := granted["wording"]; got != "I agree to receive product news." {
		t.Errorf("granted body wording = %v, want the authored sentence", got)
	}

	withdrawn := consentBody(purpose, demoConsent{State: "withdrawn", Wording: "I agree to receive product news."})
	if _, present := withdrawn["wording"]; present {
		t.Error("withdrawal body carries wording; a withdrawal demonstrates nothing and must claim no screen")
	}
}

// TestConsentBodyKeepsTheFieldsTheSeederAlreadySent guards the rest of the body
// while wording is added beside it. purpose_id, new_state and source are what
// make a seeded row addressable and identifiable as seeded, and losing one of
// them to a refactor would be invisible until a demo looked wrong.
func TestConsentBodyKeepsTheFieldsTheSeederAlreadySent(t *testing.T) {
	purpose := consentPurpose{id: "22222222-2222-2222-2222-222222222222", label: "Transactional"}
	body := consentBody(purpose, demoConsent{State: "granted", Wording: "ok"})

	if body["purpose_id"] != purpose.id {
		t.Errorf("purpose_id = %v, want %q", body["purpose_id"], purpose.id)
	}
	if body["new_state"] != "granted" {
		t.Errorf("new_state = %v, want %q", body["new_state"], "granted")
	}
	if body["source"] != seedSource {
		t.Errorf("source = %v, want %q", body["source"], seedSource)
	}
}
