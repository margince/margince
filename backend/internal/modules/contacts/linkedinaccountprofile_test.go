// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The member's own profile is drawn as their LinkedIn, so only a LinkedIn
// profile may stand there. Anything else is refused before it is stored.
func TestOnlyALinkedInProfileIsRecordedAsTheMembersOwn(t *testing.T) {
	t.Parallel()
	refused := map[string]string{
		"another site":            "https://example.com/in/qa",
		"a company page":          "https://www.linkedin.com/company/acme",
		"a look-alike host":       "https://notlinkedin.com/in/qa",
		"a script in a query":     "https://evil.example/phish?x=<script>alert(1)</script>",
		"a headline":              "Lars Jankowfsky | Founder",
		"a profile with no name":  "https://www.linkedin.com/in/",
		"a scheme other than web": "ftp://www.linkedin.com/in/qa",
	}
	for name, raw := range refused {
		_, err := normalizeMyLinkedInProfileURL(raw)
		fault, ok := errors.AsType[*values.ParseError](err)
		if !ok {
			t.Errorf("%s (%q): err = %v, want a field refusal", name, raw, err)
			continue
		}
		if fault.Field != profileURLRequestField || fault.Code != linkedInProfileURLInvalid {
			t.Errorf("%s: refused as %s/%s, want %s/%s", name, fault.Field, fault.Code,
				profileURLRequestField, linkedInProfileURLInvalid)
		}
	}
}

// A paste from a browser names the same profile as the canonical spelling, so
// it is stored that way. An empty value is still the clear.
func TestAPastedProfileIsStoredInItsCanonicalSpelling(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"linkedin.com/in/qa-onboarding":                       "https://www.linkedin.com/in/qa-onboarding",
		"http://de.linkedin.com/in/qa-onboarding/?trk=public": "https://www.linkedin.com/in/qa-onboarding",
		"https://www.linkedin.com/in/qa-onboarding":           "https://www.linkedin.com/in/qa-onboarding",
		"": "",
	} {
		got, err := normalizeMyLinkedInProfileURL(raw)
		if err != nil {
			t.Errorf("%q: refused with %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("%q stored as %q, want %q", raw, got, want)
		}
	}
}

// A profile address is short; one thousands of characters long is refused
// with the limit named, so the reader learns what to shorten.
func TestAnOverlongProfileIsRefusedWithItsLimit(t *testing.T) {
	t.Parallel()
	raw := "https://www.linkedin.com/in/" + strings.Repeat("a", maxLinkedInProfileURLLength)
	_, err := normalizeMyLinkedInProfileURL(raw)
	fault, ok := errors.AsType[*values.ParseError](err)
	if !ok || fault.Code != codeTooLong || fault.Max != maxLinkedInProfileURLLength {
		t.Fatalf("err = %#v, want too_long naming max %d", err, maxLinkedInProfileURLLength)
	}
}
