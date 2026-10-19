// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// entryUnitSource is a unit declaring the given Entries elements.
func entryUnitSource(entries string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		Entries: []extension.Entry{
` + entries + `
		},
	}
}
`
}

func entryLine(slot, target, object string) string {
	return "\t\t\t{Slot: extension." + slot + `, Label: "extX.visit", Icon: extension.IconMapPin, Target: "` + target + `", Object: "` + object + "\"},\n"
}

// gatedVerb is an operation of unit x gated on object.
func gatedVerb(object string) declaredVerb {
	v := syntheticVerb("x", "x_read", "auto_execute", "read")
	v.verb.RbacObject = object
	return v
}

func TestEntriesDeriveIntoTheManifestSorted(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", entryUnitSource(
		entryLine("SlotLogActivity", "", "ext_x_visit")+entryLine("SlotContactHeader", "report", "ext_x_visit"),
	), gatedVerb("ext_x_visit"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	if !strings.Contains(s, `"slot": "record_header.contact"`) || !strings.Contains(s, `"target": "report"`) {
		t.Fatalf("the manifest misses the declared entry:\n%s", s)
	}
	if strings.Index(s, "log_activity") > strings.Index(s, "record_header.contact") {
		t.Errorf("entries are not sorted by slot:\n%s", s)
	}
}

func TestAnEntryGatedOnAnObjectNoOperationGatesOnIsRefused(t *testing.T) {
	_, err := deriveSynthetic(t, "x", entryUnitSource(entryLine("SlotLogActivity", "", "ext_x_other")), gatedVerb("ext_x_visit"))
	if err == nil || !strings.Contains(err.Error(), "none of the unit's operations gates on") {
		t.Fatalf("err = %v, want the ungated-object refusal", err)
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestAnEntryTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, entry := range map[string]string{
		"another unit's object": entryLine("SlotLogActivity", "", "ext_y_visit"),
		"a target with a slash": entryLine("SlotLogActivity", "a/b", "ext_x_visit"),
		"one slot and view twice": entryLine("SlotLogActivity", "", "ext_x_visit") +
			entryLine("SlotLogActivity", "", "ext_x_visit"),
		"an unknown field": "\t\t\t{Slot: extension.SlotLogActivity, Label: \"extX.visit\", Icon: extension.IconMapPin, Object: \"ext_x_visit\", Style: \"red\"},\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", entryUnitSource(entry), gatedVerb("ext_x_visit"), gatedVerb("ext_y_visit")); err == nil {
				t.Fatal("generation accepted an entry the boot refuses")
			}
		})
	}
}

func TestAnEntryLabelMustBeTheUnitsOwnCopyInEveryLocale(t *testing.T) {
	manifest := func(label string) []derivedManifest {
		return []derivedManifest{{
			Unit:     extensionUnit{Name: "x"},
			Manifest: unitManifest{Entries: []declaredEntry{{Slot: "log_activity", Label: label}}},
		}}
	}
	full := map[string]map[string]string{
		"en": {"extX.visit": "Visit"}, "de": {"extX.visit": "Besuch"}, "vi": {"extX.visit": "Chuyến thăm"},
	}
	if err := checkEntryLabels(manifest("extX.visit"), full); err != nil {
		t.Fatalf("a label in every locale was refused: %v", err)
	}
	if err := checkEntryLabels(manifest("extY.visit"), full); err == nil {
		t.Error("a label outside the unit's copy namespace was accepted")
	}
	noVietnamese := map[string]map[string]string{"en": full["en"], "de": full["de"], "vi": {}}
	if err := checkEntryLabels(manifest("extX.visit"), noVietnamese); err == nil || !strings.Contains(err.Error(), "vi.json") {
		t.Errorf("err = %v, want a refusal naming the locale without the label", err)
	}
}
