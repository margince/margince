// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func TestSignalTypesDeriveIntoTheManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", reportUnitSource(`
		SignalTypes: []extension.SignalTypeDeclaration{{
			Kind: "loss", Label: "Loss reported",
			Evidence: []extension.SignalEvidenceField{
				{Name: "amount", Label: "Amount", Type: extension.SignalFieldNumber, Required: true},
				{Name: "reporter", Label: "Reporter", Type: extension.SignalFieldText, Personal: true},
			},
		}},`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{`"signal_types"`, `"kind": "loss"`, `"type": "number"`, `"personal": true`} {
		if !strings.Contains(s, want) {
			t.Errorf("the manifest misses %s:\n%s", want, s)
		}
	}
}

func TestASignalTypeTheReaderCannotDeriveIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"a computed kind":      `SignalTypes: []extension.SignalTypeDeclaration{{Kind: name(), Label: "L"}},`,
		"a kind used twice":    `SignalTypes: []extension.SignalTypeDeclaration{{Kind: "a", Label: "L"}, {Kind: "a", Label: "M"}},`,
		"a computed flag":      `SignalTypes: []extension.SignalTypeDeclaration{{Kind: "a", Label: "L", Evidence: []extension.SignalEvidenceField{{Name: "n", Label: "N", Type: extension.SignalFieldText, Personal: flag()}}}},`,
		"a type from nowhere":  `SignalTypes: []extension.SignalTypeDeclaration{{Kind: "a", Label: "L", Evidence: []extension.SignalEvidenceField{{Name: "n", Label: "N", Type: kind()}}}},`,
		"an unknown field key": `SignalTypes: []extension.SignalTypeDeclaration{{Kind: "a", Label: "L", Extra: 1}},`,
	} {
		if _, err := deriveSynthetic(t, "x", reportUnitSource(body)); err == nil {
			t.Errorf("%s: generation accepted what it cannot derive", name)
		}
	}
}
