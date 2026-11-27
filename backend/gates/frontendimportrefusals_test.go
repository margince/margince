// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The browser words an import refusal from its reason and params. A reason the
// server sends with no wording falls back to a generic sentence. A param no
// wording reads is a value the reader is never told. A reason or param only
// the browser knows is a wording no refusal reaches. Both directions fail.

import (
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/importwire"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendImportRefusals = "../frontend/src/features/import/import.notices.tsx"

var (
	tsRefusalKey    = regexp.MustCompile(`"([a-z_]+/[a-z_]+)":\s*\{`)
	tsRefusalReason = regexp.MustCompile(`reason:\s*"([a-z_]+)"`)
	tsRefusalNeeds  = regexp.MustCompile(`needs:\s*\[([^\]]*)\]`)
	tsQuotedName    = regexp.MustCompile(`"([a-z_]+)"`)
	tsRefusalInFile = regexp.MustCompile(`inFile:\s*true`)
)

func TestTheFrontendImportRefusalWordingsMatchTheServerVocabulary(t *testing.T) {
	t.Parallel()
	table := frontendRefusalsTable(t)
	reasons, needs := map[string]bool{}, map[string]bool{}
	for _, m := range tsRefusalReason.FindAllStringSubmatch(table, -1) {
		reasons[m[1]] = true
	}
	for _, m := range tsRefusalNeeds.FindAllStringSubmatch(table, -1) {
		for _, name := range tsQuotedName.FindAllStringSubmatch(m[1], -1) {
			needs[name[1]] = true
		}
	}
	// The browser adds `max` from the field error itself, not from its params.
	delete(needs, "max")
	keys := tsRefusalKey.FindAllStringSubmatch(table, -1)
	if len(keys) == 0 || len(reasons) == 0 || len(needs) == 0 {
		t.Fatalf("%s: read %d refusals, %d reasons and %d params; a gate that reads nothing agrees with everything",
			frontendImportRefusals, len(keys), len(reasons), len(needs))
	}
	read := func(param string) bool {
		inFile := param == "file" && tsRefusalInFile.MatchString(table)
		return needs[param] || inFile || param == "reason"
	}
	holdBothWays(t, "reason", importwire.FileReasons(), reasons, func(reason string) bool { return reasons[reason] })
	holdBothWays(t, "param", importwire.RefusalParams(), needs, read)
}

// holdBothWays fails a server name the browser does not read, and a browser
// name the server never sends.
func holdBothWays(t *testing.T, what string, server []string, browser map[string]bool, reads func(string) bool) {
	t.Helper()
	for _, name := range server {
		if !reads(name) {
			t.Errorf("the server sends import refusal %s %q and no wording in %s reads it", what, name, frontendImportRefusals)
		}
	}
	for name := range browser {
		if !slices.Contains(server, name) {
			t.Errorf("%s words import refusal %s %q, which the server never sends", frontendImportRefusals, what, name)
		}
	}
}

// frontendRefusalsTable is the REFUSALS literal, comments removed.
func frontendRefusalsTable(t *testing.T) string {
	t.Helper()
	source, err := gatekit.ReadFile(frontendImportRefusals)
	if err != nil {
		t.Fatalf("reading the frontend refusal table: %v", err)
	}
	const marker = "const REFUSALS: Readonly<Record<string, Refusal>> = {"
	start := indexAfter(string(source), marker)
	if start < 0 {
		t.Fatalf("%s no longer declares REFUSALS as an object literal", frontendImportRefusals)
	}
	end := indexAfter(string(source)[start:], "\n};")
	if end < 0 {
		t.Fatalf("%s's REFUSALS literal is unterminated", frontendImportRefusals)
	}
	return gatekit.TSWithoutComments(string(source)[start : start+end])
}
