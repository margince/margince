// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestAnEntityLanguageIsStoredLowerCaseAndTwoLetters(t *testing.T) {
	if got, err := cleanEntityLanguage("DE"); err != nil || got != "de" {
		t.Errorf("DE cleaned to %q, %v; want de", got, err)
	}
	_, err := cleanEntityLanguage("fra")
	if fault, ok := errors.AsType[*values.ParseError](err); !ok || fault.Field != wireDefaultLanguage || fault.Code != "invalid_language" {
		t.Errorf("fra answered %v, want invalid_language on %s", err, wireDefaultLanguage)
	}
}
