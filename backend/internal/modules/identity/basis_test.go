// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
)

// What the installation is measured in is ASKED on the path that has a human
// and DEFAULTED on the path that does not.
//
// A deployment file has nobody at boot, so an omitted key must still start the
// installation. A claim has an operator in front of a form that asks, so an
// absent value there means a client that stopped asking — and defaulting it
// would put the installation permanently on EUR because nobody was ever shown
// the question. The base currency stops being changeable once anything
// converts against it, so that window closes on its own.

func TestAClaimMustNameWhatTheInstallationIsMeasuredIn(t *testing.T) {
	cases := []struct {
		name     string
		currency string
		language string
		field    string
		says     string
	}{
		{"no currency at all", "", "en", "installation.base_currency", "ISO-4217"},
		{"a currency nobody issues", "EURO", "en", "installation.base_currency", "ISO-4217"},
		{"a lowercase currency", "chf", "en", "installation.base_currency", "ISO-4217"},
		{"no language at all", "EUR", "", "installation.base_language", "en, de, vi"},
		{"a language we cannot write", "EUR", "fr", "installation.base_language", "en, de, vi"},
		{"a locale tag, not a language", "EUR", "de-DE", "installation.base_language", "en, de, vi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := resolveBasis(InstallationBootstrap{
				BaseCurrency: tc.currency,
				BaseLanguage: tc.language,
			}, originClaimed)
			if err == nil {
				t.Fatal("accepted; the installation would have been given a basis nobody chose")
			}
			// The claim form shows a refusal against the field it belongs to,
			// so one that names no field lands at the bottom of the page
			// pointing at nothing.
			var fault apperrors.FieldFault
			if !errors.As(err, &fault) {
				t.Fatalf("refusal is %T and names no field for the form to attach it to", err)
			}
			field, _, message := fault.FieldFault()
			if field != tc.field {
				t.Errorf("refusal names %q, want %q", field, tc.field)
			}
			// And it has to say what WOULD work: the operator is choosing a
			// value, not being told they chose wrong.
			if !strings.Contains(message, tc.says) {
				t.Errorf("refusal says %q, which never mentions %q", message, tc.says)
			}
		})
	}
}

func TestAClaimNamingItsBasisKeepsIt(t *testing.T) {
	// The admit case, and it is the one that matters: every refusal test above
	// would also pass against a resolveBasis that refused everything.
	currency, language, err := resolveBasis(InstallationBootstrap{
		BaseCurrency: "CHF",
		BaseLanguage: "de",
	}, originClaimed)
	if err != nil {
		t.Fatalf("a claim naming CHF and German was refused: %v", err)
	}
	if currency != "CHF" || language != "de" {
		t.Errorf("resolved to %s/%s, want CHF/de — the operator's answer is what the installation gets", currency, language)
	}
}

func TestAConfiguredBootstrapStillStartsWithoutABasis(t *testing.T) {
	// A deployment file that omits both keys has to boot. The operator fixes it
	// in Settings afterwards; refusing here would strand an installation that
	// upgraded into this build with an older config.
	currency, language, err := resolveBasis(InstallationBootstrap{}, originConfigured)
	if err != nil {
		t.Fatalf("a deployment file with no basis refused to boot: %v", err)
	}
	if currency != "EUR" || language != "en" {
		t.Errorf("fell back to %s/%s, want EUR/en", currency, language)
	}
}

func TestAConfiguredBootstrapKeepsWhatTheFileNamed(t *testing.T) {
	// The fallback must not overwrite a configured answer — that would make the
	// deployment file's currency key decorative.
	currency, language, err := resolveBasis(InstallationBootstrap{
		BaseCurrency: "VND",
		BaseLanguage: "vi",
	}, originConfigured)
	if err != nil {
		t.Fatalf("a configured basis was refused: %v", err)
	}
	if currency != "VND" || language != "vi" {
		t.Errorf("resolved to %s/%s, want VND/vi", currency, language)
	}
}
