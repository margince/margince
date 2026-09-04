// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"strings"
	"testing"
)

// The provider list is validated for the one thing a blank key would cause: a
// silent no-op. An empty string matches no provider, so a list carrying one
// would report itself saved while enabling nothing.
func TestEnabledOidcProvidersRefusesABlankKey(t *testing.T) {
	if err := EnabledOidcProviders.ValidateJSON([]byte(`["google", ""]`)); err == nil {
		t.Error("a blank provider key was accepted; it would save cleanly and enable nothing")
	}
	if err := EnabledOidcProviders.ValidateJSON([]byte(`["google", "microsoft"]`)); err != nil {
		t.Errorf("a list of real keys was refused: %v", err)
	}
	// Whitespace matches no provider, so accepting it would save cleanly and
	// enable nothing — a setting that reports itself applied and is not.
	if err := EnabledOidcProviders.ValidateJSON([]byte(`[" google"]`)); err == nil {
		t.Error("a padded provider key was accepted; it would match no provider")
	}
	// Empty is a CHOICE — offer password only — and must not be confused with a
	// malformed list.
	if err := EnabledOidcProviders.ValidateJSON([]byte(`[]`)); err != nil {
		t.Errorf("choosing no provider was refused: %v", err)
	}
}

// The organization name's bounds: non-empty, and at most the entry's ceiling.
//
// Counted in RUNES, so a name of CJK characters is measured in characters rather
// than in the bytes they encode to.
func TestInstallationNameIsBoundedInCharacters(t *testing.T) {
	if err := Name.ValidateJSON([]byte(`"Acme GmbH"`)); err != nil {
		t.Errorf("an ordinary organization name was refused: %v", err)
	}
	if err := Name.ValidateJSON([]byte(`"   "`)); err == nil {
		t.Error("a blank name was accepted; an installation with no name renders as nothing everywhere")
	}

	atCeiling := `"` + strings.Repeat("a", maxInstallationNameLen) + `"`
	if err := Name.ValidateJSON([]byte(atCeiling)); err != nil {
		t.Errorf("a name AT the ceiling was refused, so the bound is off by one: %v", err)
	}
	overCeiling := `"` + strings.Repeat("a", maxInstallationNameLen+1) + `"`
	if err := Name.ValidateJSON([]byte(overCeiling)); err == nil {
		t.Errorf("a name of %d characters was accepted", maxInstallationNameLen+1)
	}

	// The rune half: this is under the ceiling in characters and over it in
	// bytes, so a byte-counted bound would refuse a name a customer may have.
	cjk := `"` + strings.Repeat("組", maxInstallationNameLen-1) + `"`
	if err := Name.ValidateJSON([]byte(cjk)); err != nil {
		t.Errorf("a %d-character CJK name was refused, so the bound is counting bytes: %v",
			maxInstallationNameLen-1, err)
	}
}
