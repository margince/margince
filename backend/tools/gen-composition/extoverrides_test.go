// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func writeOverride(t *testing.T, dir, file, body string) {
	t.Helper()
	layer := filepath.Join(dir, frontendLayer, overridesLayer)
	if err := os.MkdirAll(layer, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layer, file), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// An override may cover one language: a unit that only knows its customer's
// English wording must not be forced to invent German.
func TestOverridesMayCoverOnlySomeLanguages(t *testing.T) {
	dir := t.TempDir()
	writeOverride(t, dir, "en.json", `{"nav.deals":"Opportunities"}`)
	got, err := collectUnitOverrides("acme", dir)
	if err != nil || len(got) != 1 || got[0].Locale != "en" {
		t.Fatalf("got %#v, err %v; want one en file accepted", got, err)
	}
}

func TestOverridesRefuseALocaleCoreDoesNotShip(t *testing.T) {
	dir := t.TempDir()
	writeOverride(t, dir, "fr.json", `{}`)
	if _, err := collectUnitOverrides("acme", dir); err == nil || !strings.Contains(err.Error(), "does not ship") {
		t.Fatalf("err = %v, want the unknown-locale refusal", err)
	}
}

func TestOverridesRefuseAFileThatIsNotALocaleJSON(t *testing.T) {
	dir := t.TempDir()
	writeOverride(t, dir, "notes.txt", `x`)
	if _, err := collectUnitOverrides("acme", dir); err == nil || !strings.Contains(err.Error(), "<locale>.json") {
		t.Fatalf("err = %v, want the file-name refusal", err)
	}
}

func TestOverridesRefuseAValueThatIsNotAString(t *testing.T) {
	dir := t.TempDir()
	writeOverride(t, dir, "en.json", `{"nav.deals":3}`)
	if _, err := collectUnitOverrides("acme", dir); err == nil {
		t.Fatal("a numeric override was accepted")
	}
}

func TestOverridesAreAbsentWhenTheFolderIs(t *testing.T) {
	got, err := collectUnitOverrides("acme", t.TempDir())
	if err != nil || got != nil {
		t.Fatalf("got %#v, err %v; want nothing and no error", got, err)
	}
}

func TestMergeOverridesRefusesAKeyCoreDoesNotHave(t *testing.T) {
	_, err := mergeUnitOverrides([]unitOverride{
		{Unit: "acme", Locale: "en", Keys: map[string]string{"nav.nope": "x"}},
	}, map[string]bool{"nav.deals": true})
	if err == nil || !strings.Contains(err.Error(), "nav.nope") {
		t.Fatalf("err = %v, want the unknown-key refusal naming the key", err)
	}
}

func TestMergeOverridesRefusesTwoUnitsOnOneKeyAndLocale(t *testing.T) {
	core := map[string]bool{"nav.deals": true}
	_, err := mergeUnitOverrides([]unitOverride{
		{Unit: "alpha", Locale: "en", Keys: map[string]string{"nav.deals": "A"}},
		{Unit: "beta", Locale: "en", Keys: map[string]string{"nav.deals": "B"}},
	}, core)
	if err == nil || !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("err = %v, want both units named", err)
	}
	// The same key in another language is no collision.
	if _, err := mergeUnitOverrides([]unitOverride{
		{Unit: "alpha", Locale: "en", Keys: map[string]string{"nav.deals": "A"}},
		{Unit: "beta", Locale: "de", Keys: map[string]string{"nav.deals": "B"}},
	}, core); err != nil {
		t.Fatalf("different languages refused: %v", err)
	}
}

// Consent, erasure, approval and sign-in wording are overridable: no key is
// exempt from the check.
func TestMergeOverridesAcceptsEveryCoreKey(t *testing.T) {
	core, err := coreCatalogueKeys(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{}
	for k := range core {
		keys[k] = "x"
	}
	if _, err := mergeUnitOverrides([]unitOverride{{Unit: "acme", Locale: "en", Keys: keys}}, core); err != nil {
		t.Fatalf("a core key was refused: %v", err)
	}
}

// A reader that sees fewer keys than en.ts has would refuse a real key, so the
// count comes from an independent pattern.
func TestCoreCatalogueKeysSeeEveryKey(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	core, err := coreCatalogueKeys(root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "frontend/src/i18n/en.ts"))
	if err != nil {
		t.Fatal(err)
	}
	want := len(regexp.MustCompile(`(?m)^ {2}"[^"\n]+":`).FindAll(raw, -1))
	if want == 0 || len(core) != want {
		t.Fatalf("reader saw %d keys, en.ts has %d", len(core), want)
	}
}

func TestOverridesEmitSortedAndStubStaysEmpty(t *testing.T) {
	got := string(extLocalesGen(nil, map[string]map[string]string{
		"en": {"b.key": "B", "a.key": "A"},
	}))
	if !strings.Contains(got, "export const extensionOverrides = {\n  \"en\": {\n    \"a.key\": \"A\",\n    \"b.key\": \"B\",") {
		t.Fatalf("overrides not emitted sorted:\n%s", got)
	}
	if empty := string(extLocalesGen(nil, nil)); !strings.Contains(empty, "export const extensionOverrides = {};") {
		t.Fatalf("empty stub lacks the empty overrides export:\n%s", empty)
	}
}
