// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestContactLinkedInRefusesACompanyPage(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"https://www.linkedin.com/company/acme",
		"https://www.linkedin.com/company/acme/about",
		"https://www.linkedin.com/school/acme-university",
		"https://www.linkedin.com/showcase/acme-labs",
		"https://www.linkedin.com/in/",
		"https://www.linkedin.com/",
	} {
		_, err := NormalizeContactLinkedInURL(raw)
		parseErr, ok := errors.AsType[*values.ParseError](err)
		if !ok || parseErr.Code != "linkedin_url_not_profile" {
			t.Errorf("NormalizeContactLinkedInURL(%q) = %v, want linkedin_url_not_profile", raw, err)
		}
	}
}

func TestContactLinkedInAcceptsAProfileOnACountrySubdomain(t *testing.T) {
	t.Parallel()
	got, err := NormalizeContactLinkedInURL("https://de.linkedin.com/in/sam-example/")
	if err != nil || got != "https://www.linkedin.com/in/sam-example" {
		t.Fatalf("got %q, %v, want the canonical profile", got, err)
	}
}

func TestContactLinkedInFoldsEveryLinkedInHostSpelling(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"linkedin.com/in/sam-example",
		"https://www.linkedin.com/in/sam-example",
		"https://de.linkedin.com/in/sam-example",
		"http://m.linkedin.com/in/sam-example/",
	} {
		got, err := NormalizeContactLinkedInURL(raw)
		if err != nil {
			t.Fatalf("NormalizeContactLinkedInURL(%q): %v", raw, err)
		}
		if got != "https://www.linkedin.com/in/sam-example" {
			t.Errorf("NormalizeContactLinkedInURL(%q) = %q", raw, got)
		}
	}
}

func TestACompanyLinkedInStillTakesACompanyPage(t *testing.T) {
	t.Parallel()
	got, err := NormalizeCompanyLinkedInURL("https://www.linkedin.com/company/acme/about")
	if err != nil || got != "https://www.linkedin.com/company/acme" {
		t.Fatalf("got %q, %v, want the company page", got, err)
	}
}
