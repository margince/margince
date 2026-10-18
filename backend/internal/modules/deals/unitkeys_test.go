// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The server writes unit keys that only the client turns into words. A key the
// client's list lacks reaches the reader as the raw key, so every key the
// server writes must be on that list.

const clientUnitKeys = "../../../../frontend/src/features/deals/productunit.tsx"

var (
	unitKeysLiteral = regexp.MustCompile(`UNIT_KEYS = \[([^\]]*)\]`)
	quotedWord      = regexp.MustCompile(`"([a-z_]+)"`)
	mappedUnitKey   = regexp.MustCompile(`\('[^']+', '([a-z_]+)'\)`)
	columnDefault   = regexp.MustCompile(`ALTER COLUMN unit SET DEFAULT '([a-z_]+)'`)
)

func readClientUnitKeys(t *testing.T) map[string]bool {
	t.Helper()
	source, err := os.ReadFile(clientUnitKeys)
	if err != nil {
		t.Fatalf("reading the client's unit list: %v", err)
	}
	literal := unitKeysLiteral.FindSubmatch(source)
	if literal == nil {
		t.Fatalf("%s no longer declares UNIT_KEYS as an array literal", clientUnitKeys)
	}
	keys := map[string]bool{}
	for _, m := range quotedWord.FindAllSubmatch(literal[1], -1) {
		keys[string(m[1])] = true
	}
	if len(keys) == 0 {
		t.Fatal("no keys parsed out of UNIT_KEYS; a list read as empty agrees with nothing")
	}
	return keys
}

func TestTheDefaultUnitIsAKeyTheClientTranslates(t *testing.T) {
	t.Parallel()
	keys := readClientUnitKeys(t)
	if !keys[defaultUnit] {
		t.Fatalf("defaultUnit %q is not in the client's UNIT_KEYS %v", defaultUnit, keys)
	}
	unit, _, _ := normalizeLineDefaults(nil, nil, nil)
	if unit != defaultUnit {
		t.Fatalf("a line with no unit got %q, want %q", unit, defaultUnit)
	}
}

func TestTheUnitMigrationMapsOnlyToKeysTheClientTranslates(t *testing.T) {
	t.Parallel()
	keys := readClientUnitKeys(t)
	matches, err := filepath.Glob("../../../migrations/core/*_a_unit_is_a_key_the_reader_sees_translated.up.sql")
	if err != nil || len(matches) != 1 {
		t.Fatalf("want exactly one unit-key migration, got %v (%v)", matches, err)
	}
	source, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("reading %s: %v", matches[0], err)
	}
	targets := mappedUnitKey.FindAllSubmatch(source, -1)
	targets = append(targets, columnDefault.FindAllSubmatch(source, -1)...)
	if len(targets) < len(keys) {
		t.Fatalf("parsed only %d unit keys out of %s; the parser no longer reads its shape", len(targets), matches[0])
	}
	for _, m := range targets {
		if !keys[string(m[1])] {
			t.Errorf("%s writes unit %q, which the client's UNIT_KEYS does not translate", matches[0], m[1])
		}
	}
}
