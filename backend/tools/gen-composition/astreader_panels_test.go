// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// panelUnitSource is a unit declaring the given Panels elements.
func panelUnitSource(panels string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		Panels: []extension.Panel{
` + panels + `
		},
	}
}
`
}

func panelLine(export, object string) string {
	return "\t\t\t{Slot: extension.SlotDealPanel, Export: \"" + export + "\", Object: \"" + object + "\"},\n"
}

func TestPanelsDeriveIntoTheManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", panelUnitSource(panelLine("DealPanel", "ext_x_visit")), gatedVerb("ext_x_visit"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{`"slot": "record_panel.deal"`, `"export": "DealPanel"`, `"object": "ext_x_visit"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the manifest misses %s:\n%s", want, s)
		}
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestAPanelTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, panel := range map[string]string{
		"an ungated object":     panelLine("DealPanel", "ext_x_other"),
		"another unit's object": panelLine("DealPanel", "ext_y_visit"),
		"a lower-case export":   panelLine("dealPanel", "ext_x_visit"),
		"one slot twice":        panelLine("DealPanel", "ext_x_visit") + panelLine("OtherPanel", "ext_x_visit"),
		"an unknown field":      "\t\t\t{Slot: extension.SlotDealPanel, Export: \"DealPanel\", Object: \"ext_x_visit\", Width: \"wide\"},\n",
		"an entry slot":         "\t\t\t{Slot: extension.SlotDealHeader, Export: \"DealPanel\", Object: \"ext_x_visit\"},\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", panelUnitSource(panel), gatedVerb("ext_x_visit"), gatedVerb("ext_y_visit")); err == nil {
				t.Fatal("generation accepted a panel the boot refuses")
			}
		})
	}
}

func TestAPanelNeedsTheUnitToShipAFrontend(t *testing.T) {
	manifests := []derivedManifest{{Unit: extensionUnit{Name: "x"}, Manifest: unitManifest{
		Panels: []declaredPanel{{Slot: "record_panel.deal", Export: "DealPanel", Object: "ext_x_visit"}},
	}}}
	if err := checkPanelFrontends(manifests); err == nil || !strings.Contains(err.Error(), "ships no frontend/package.json") {
		t.Fatalf("err = %v, want the missing-frontend refusal", err)
	}
	manifests[0].Unit.Frontend = &unitFrontend{Package: "@margince-ext/x", Export: "@margince-ext/x"}
	if err := checkPanelFrontends(manifests); err != nil {
		t.Fatalf("a panel with a frontend was refused: %v", err)
	}
}
