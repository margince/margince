// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// TestTheDefaultEntityTakesNoCountryOfItsOwn: the default entity's country is
// the installation's jurisdiction, so a country sent for it is refused and a
// further entity keeps its own.
func TestTheDefaultEntityTakesNoCountryOfItsOwn(t *testing.T) {
	t.Parallel()
	country := "vn"
	_, _, err := legalEntityPatch(LegalEntity{IsDefault: true}, LegalEntityInput{Country: &country})
	if fault, ok := errors.AsType[*values.ParseError](err); !ok || fault.Code != codeFieldOnInstallation || fault.Field != wireCountry {
		t.Errorf("a country on the default entity answered %v, want %s on %s", err, codeFieldOnInstallation, wireCountry)
	}
	if _, changed, err := legalEntityPatch(LegalEntity{}, LegalEntityInput{Country: &country}); err != nil || len(changed) != 1 {
		t.Errorf("a country on a further entity answered %v, changed %v; want it stored", err, changed)
	}
}

// A document language names a language: "zz" has the shape and names none.
func TestAnEntityDocumentLanguageIsOneTheRegistryAssigns(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{"DE": "de", "vi": "vi", "zz": "", "xx": "", "deu": ""} {
		got, err := cleanEntityLanguage(raw)
		if want == "" {
			if fault, ok := errors.AsType[*values.ParseError](err); !ok || fault.Field != wireDefaultLanguage {
				t.Errorf("default_language %q answered %q, %v; want a refusal on %s", raw, got, err, wireDefaultLanguage)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("default_language %q answered %q, %v; want %q", raw, got, err, want)
		}
	}
}
