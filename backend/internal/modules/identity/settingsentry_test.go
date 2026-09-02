// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "testing"

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
