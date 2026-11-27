// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

func TestEveryCoreApplicationHasARegisteredSetting(t *testing.T) {
	registered := map[string]bool{}
	for _, definition := range Definitions() {
		registered[definition.Key()] = true
	}
	for _, provider := range AppProviders() {
		kind, err := appKindFor(provider)
		if err != nil {
			t.Fatal(err)
		}
		if kind.entry == nil || !registered[kind.entry.Key()] {
			t.Errorf("%s has no registered application setting", provider)
		}
	}
}
