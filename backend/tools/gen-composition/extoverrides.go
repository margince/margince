// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// A unit's wording for core's own keys, merged at build time.
//
// This is not the unit's own copy (extlocales.go). That copy is namespaced to the
// unit and covers every language. An override names a core key and may cover one.
// The installation owns the wording of every key it overrides.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// overridesLayer is the directory inside a unit's frontend package holding its
// overrides, one file per locale, apart from the unit's own copy.
const overridesLayer = "i18n-overrides"

// unitOverride is one unit's overrides for one locale.
type unitOverride = unitLocale

// coreCatalogueKey matches one key line of en.ts. The catalogue is one entry per
// key at two-space indent, so a value's continuation line never matches.
var coreCatalogueKey = regexp.MustCompile(`(?m)^ {2}"([^"\n]+)":`)

// collectUnitOverrides reads a unit's override layer. Absent is fine.
func collectUnitOverrides(name, dir string) ([]unitOverride, error) {
	return readLocaleFiles(name, filepath.Join(dir, frontendLayer, overridesLayer))
}

// coreCatalogueKeys reads the key set of the core catalogue. Every locale
// mirrors en.ts's keys, so one file answers for all of them.
func coreCatalogueKeys(root string) (map[string]bool, error) {
	raw, err := os.ReadFile(filepath.Join(root, "frontend", "src", "i18n", "en.ts")) // #nosec G304 -- the tree this composer was pointed at
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for _, m := range coreCatalogueKey.FindAllSubmatch(raw, -1) {
		keys[string(m[1])] = true
	}
	return keys, nil
}

// mergeUnitOverrides folds every unit's overrides into one map per locale. No
// key is exempt: consent, erasure, approval and sign-in wording included.
func mergeUnitOverrides(overrides []unitOverride, coreKeys map[string]bool) (map[string]map[string]string, error) {
	merged := map[string]map[string]string{}
	claimedBy := map[string]string{}
	for _, o := range overrides {
		for _, key := range sortedKeys(o.Keys) {
			if !coreKeys[key] {
				return nil, fmt.Errorf("extensions/%s: %s/%s/%s.json overrides %q, which the core catalogue does not have — fix the key or remove it",
					o.Unit, frontendLayer, overridesLayer, o.Locale, key)
			}
			// Two owners of one wording would make the winner depend on the
			// order the filesystem was read in.
			id := o.Locale + "\x00" + key
			if prev, taken := claimedBy[id]; taken && prev != o.Unit {
				return nil, fmt.Errorf("core key %q (%s) is overridden by both %s and %s — one key, one owner",
					key, o.Locale, prev, o.Unit)
			}
			claimedBy[id] = o.Unit
			if merged[o.Locale] == nil {
				merged[o.Locale] = map[string]string{}
			}
			merged[o.Locale][key] = o.Keys[key]
		}
	}
	return merged, nil
}

// composedOverrides collects and merges every unit's overrides, reading the core
// catalogue only when some unit has any, so a tree with none needs no frontend.
func composedOverrides(root string, units []extensionUnit) (map[string]map[string]string, error) {
	var all []unitOverride
	for _, u := range units {
		supplied, err := collectUnitOverrides(u.Name, u.Dir)
		if err != nil {
			return nil, err
		}
		all = append(all, supplied...)
	}
	if len(all) == 0 {
		return nil, nil
	}
	core, err := coreCatalogueKeys(root)
	if err != nil {
		return nil, err
	}
	return mergeUnitOverrides(all, core)
}

// writeLocaleObject emits one `{ locale: { key: value } }` object in sorted
// order, so the file is a function of the enabled set.
func writeLocaleObject(b *strings.Builder, name string, merged map[string]map[string]string) {
	if len(merged) == 0 {
		fmt.Fprintf(b, "\nexport const %s = {};\n", name)
		return
	}
	fmt.Fprintf(b, "\nexport const %s = {\n", name)
	locales := make([]string, 0, len(merged))
	for locale := range merged {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	for _, locale := range locales {
		fmt.Fprintf(b, "  %s: {\n", tsString(locale))
		for _, key := range sortedKeys(merged[locale]) {
			fmt.Fprintf(b, "    %s: %s,\n", tsString(key), tsString(merged[locale][key]))
		}
		b.WriteString("  },\n")
	}
	b.WriteString("};\n")
}
