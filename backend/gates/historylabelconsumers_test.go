// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"github.com/margince/margince/backend/internal/compose"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// History also reads stored column labels, the address dialog and company profile.
// These maps already translate a key; adding another synthetic entry would shadow them.
func historyAuxiliaryLabels(t *testing.T, source string) map[string]bool {
	t.Helper()
	out := tsMapKeys(t, historyLabelSource, source, "const STORED_COLUMN_LABELS = new Map<string, MessageKey>([")
	address, err := gatekit.ReadFile("../frontend/src/lib/addresscolumns.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`\["([^"]+)",\s*"[^"]+"\]`).FindAllStringSubmatch(string(address), -1) {
		out[match[1]] = true
	}
	profile, err := gatekit.ReadFile("../frontend/src/format/companyprofilelabels.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`(?m)^\s*([a-z_]+):\s*"[^"]+",`).FindAllStringSubmatch(string(profile), -1) {
		out[match[1]] = true
	}
	if len(out) < 20 {
		t.Fatalf("read only%d auxiliary History words", len(out))
	}
	return out
}

func historySettingObjects(t *testing.T) []string {
	t.Helper()
	catalog := compose.SettingsCatalogForTest()
	if len(catalog) < 50 {
		t.Fatalf("read only %d registered settings", len(catalog))
	}
	var objects []string
	for _, setting := range catalog {
		objects = append(objects, setting.Object)
	}
	return objects
}

func TestHistoryLabelsReadCapitalAndDottedWireKeys(t *testing.T) {
	source := `const LABELS = new Map<string, MessageKey>([["FullName","full"],["social.linkedin","linkedin"]]);`
	keys := tsMapKeys(t, "planted.ts", source, "const LABELS = new Map<string, MessageKey>([")
	if !keys["FullName"] || !keys["social.linkedin"] {
		t.Fatalf("lost raw audit keys: %v", keys)
	}
}

func historyProjectedSettingsKeys(t *testing.T) []string {
	t.Helper()
	_, entities := fieldHistoryProjection(t)
	var keys []string
	for _, setting := range compose.SettingsCatalogForTest() {
		if entities[setting.Object] {
			keys = append(keys, setting.Key)
		}
	}
	return keys
}
