// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// instrumentationUnitSource is a unit declaring the given Instrumentation field.
func instrumentationUnitSource(field string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		` + field + `
	}
}
`
}

func TestInstrumentationDerivesIntoTheManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", instrumentationUnitSource(`Instrumentation: &extension.Instrumentation{
			Start:   start,
			Browser: "instrument",
			Edge:    edge(),
		},`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{`"instrumentation"`, `"start": true`, `"browser": "instrument"`, `"edge": true`} {
		if !strings.Contains(s, want) {
			t.Errorf("the manifest misses %s:\n%s", want, s)
		}
	}
}

func TestInstrumentationRecordsOnlyThePartsItDeclares(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", instrumentationUnitSource(`Instrumentation: &extension.Instrumentation{Browser: "instrument", Start: nil},`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	if !strings.Contains(s, `"browser": "instrument"`) || strings.Contains(s, `"start"`) || strings.Contains(s, `"edge"`) {
		t.Fatalf("manifest should carry the browser part alone:\n%s", s)
	}
}

func TestNoInstrumentationOmitsTheField(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", instrumentationUnitSource(``))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(derived), "instrumentation") {
		t.Fatalf("a unit declaring none carries the field:\n%s", derived)
	}
}

func TestInstrumentationDerivationRefusals(t *testing.T) {
	for name, field := range map[string]string{
		"not a pointer literal": `Instrumentation: makeInstr(),`,
		"browser not a literal": `Instrumentation: &extension.Instrumentation{Browser: name},`,
		"browser not an export": `Instrumentation: &extension.Instrumentation{Browser: "./x.ts"},`,
		"browser is default":    `Instrumentation: &extension.Instrumentation{Browser: "default"},`,
		"unknown field":         `Instrumentation: &extension.Instrumentation{Sink: x},`,
		"declares nothing":      `Instrumentation: &extension.Instrumentation{},`,
		"only nil parts":        `Instrumentation: &extension.Instrumentation{Start: nil, Edge: nil},`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", instrumentationUnitSource(field)); err == nil {
				t.Fatal("want a refusal")
			}
		})
	}
}

func TestABrowserInstrumentNeedsTheUnitToShipAFrontend(t *testing.T) {
	manifests := []derivedManifest{{Unit: extensionUnit{Name: "x"}, Manifest: unitManifest{
		Instrumentation: &unitInstrumentation{Browser: "instrument"},
	}}}
	if err := checkInstrumentFrontends(manifests); err == nil || !strings.Contains(err.Error(), "ships no frontend/package.json") {
		t.Fatalf("err = %v, want the missing-frontend refusal", err)
	}
	manifests[0].Unit.Frontend = &unitFrontend{Package: "@margince-ext/x", Export: "@margince-ext/x"}
	if err := checkInstrumentFrontends(manifests); err != nil {
		t.Fatalf("a browser instrument with a frontend was refused: %v", err)
	}
	// A backend-only instrumentation needs no frontend.
	backend := []derivedManifest{{Unit: extensionUnit{Name: "x"}, Manifest: unitManifest{
		Instrumentation: &unitInstrumentation{Start: true},
	}}}
	if err := checkInstrumentFrontends(backend); err != nil {
		t.Fatalf("a backend-only instrumentation was refused: %v", err)
	}
}
