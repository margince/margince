// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// needsUnitSource is a unit declaring the given Needs elements.
func needsUnitSource(needs string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		Needs:       []extension.Need{` + needs + `},
	}
}
`
}

// The manifest publishes each need with its reason, sorted by capability, so
// the admin who installs the unit reads what it asks for and why.
func TestNeedsDeriveIntoManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", needsUnitSource(
		`{Capability: extension.CapabilityObjects, Reason: "Keeps files."}, {Capability: extension.CapabilityModels, Reason: "Summarises."}`))
	if err != nil {
		t.Fatal(err)
	}
	models := strings.Index(string(derived), `"capability": "models"`)
	objects := strings.Index(string(derived), `"capability": "objects"`)
	if models < 0 || objects < 0 || models > objects || !strings.Contains(string(derived), `"reason": "Summarises."`) {
		t.Errorf("derived manifest does not list both needs, models first, with reasons:\n%s", derived)
	}
}

func TestANeedTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, needs := range map[string]string{
		"unknown capability": `{Capability: "vault", Reason: "Reads tokens."}`,
		"no reason":          `{Capability: extension.CapabilityModels}`,
		"declared twice":     `{Capability: extension.CapabilityModels, Reason: "a"}, {Capability: extension.CapabilityModels, Reason: "b"}`,
	} {
		if _, err := deriveSynthetic(t, "x", needsUnitSource(needs)); err == nil {
			t.Errorf("%s: the generator published it", name)
		}
	}
}
