// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// A section present in the file is not a section in the package.
//
// gates/piicoverage_test.go decides whether the Art. 15 export reaches a PII
// table by reading the SQL literals in the assembly files. That answers "is the
// query written", not "is it assembled": delete the one `append` line that adds
// a chapter to sarSections and the query text stays where it is, so the gate
// goes on reporting the tables as covered while the export stops carrying them.
//
// Proven, not assumed — removing the append for sarCommunicationSections leaves
// that gate green while three registered tables silently leave the package.
//
// This closes the gap from the side that can see it. sarSections is unexported,
// so no gate can call it; the privacy package can, and the assembled list is the
// only thing that knows what the export will actually run.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fromJoin names the table a FROM or JOIN clause reads. SAR sections are
// SELECTs, so the write-target readers used elsewhere cannot see them.
//
// A character-identical copy of fromJoinRe in gates/piicoverage_test.go, and
// deliberately so rather than shared: backend/gates carries no non-test file, so
// it is not an importable package, and there is no test helper package in the
// tree to move it to. The two ask the same question of different halves — that
// one of the assembly files' text, this one of the assembled list — and a change
// to what counts as a read belongs in both.
var fromJoin = regexp.MustCompile(`(?is)\b(?:from|join)\s+([a-z_][a-z0-9_]*)`)

// tablesTheExportReaches assembles the real gather list and reports every table
// its statements read.
func tablesTheExportReaches(t *testing.T) map[string]bool {
	t.Helper()
	var pkg SARPackage
	sections := sarSections(&pkg, ids.New[ids.PersonKind](), []string{"subject@sar.test"}, []ids.UUID{ids.NewV7()}, []ids.UUID{ids.NewV7()})

	// A floor, for the reason the schema check beside this one carries one: an
	// empty gather list reads exactly like a complete one, and every assertion
	// below would pass over it having examined nothing.
	const atLeast = 20
	if len(sections) < atLeast {
		t.Fatalf("the gather list has %d section(s), fewer than the %d this check assumes — "+
			"it was about to pass having read almost nothing", len(sections), atLeast)
	}

	reached := map[string]bool{}
	for _, section := range sections {
		for _, m := range fromJoin.FindAllStringSubmatch(section.query, -1) {
			reached[m[1]] = true
		}
	}
	return reached
}

// registeredForExport reads the tables the PII registry marks sarRead — the
// declaration that says the Art. 15 package must carry them.
//
// Read from the gate's source rather than restated here. A hand-kept list was
// the first shape of this test and it covered nine tables against the registry's
// twenty-eight, so dropping any chapter outside those nine went unnoticed —
// which is the same under-recognition the test was written to catch, reproduced
// in the test itself.
func registeredForExport(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../../gates/piicoverage_test.go")
	if err != nil {
		t.Fatalf("reading the PII registry: %v", err)
	}

	// Entries read `"table": {… sarRead: true …}`, spanning lines.
	entry := regexp.MustCompile(`(?s)"([a-z_]+)":\s*\{([^}]*?)\}`)
	var tables []string
	for _, m := range entry.FindAllStringSubmatch(string(raw), -1) {
		if strings.Contains(m[2], "sarRead: true") {
			tables = append(tables, m[1])
		}
	}
	if len(tables) < 20 {
		t.Fatalf("the registry reports %d table(s) as sarRead, fewer than this check assumes — "+
			"it was about to pass having examined almost nothing", len(tables))
	}
	return tables
}

func TestEveryPromisedTableIsActuallyAssembled(t *testing.T) {
	reached := tablesTheExportReaches(t)
	for _, table := range registeredForExport(t) {
		if !reached[table] {
			t.Errorf("no ASSEMBLED section reads %s. Its query may still be written in the file — "+
				"which is all gates/piicoverage_test.go can see — but sarSections no longer returns "+
				"it, so the subject's package leaves without it", table)
		}
	}
}

// The credential columns stay out of the assembled statements, not merely out
// of the ones somebody remembered to look at.
//
// The coverage gate holds this too, from the file. Held here as well because the
// two questions differ: that one asks whether the withheld column is written
// anywhere in the assembly files, this one asks whether it reaches a statement
// the export will run. A section built by string concatenation would pass the
// first and could fail this.
func TestTheConfirmLinkSectionCarriesNoCredential(t *testing.T) {
	var pkg SARPackage
	sections := sarSections(&pkg, ids.New[ids.PersonKind](), []string{"subject@sar.test"}, []ids.UUID{ids.NewV7()}, []ids.UUID{ids.NewV7()})

	var probed int
	for _, section := range sections {
		if !strings.Contains(section.query, "confirm_token") {
			continue
		}
		probed++
		// The SELECT list only: `expires_at` in a CASE is how the outcome is
		// derived, and a column in a WHERE is how the subject's own rows are
		// found. Neither puts a value in the package.
		selected := section.query
		if from := regexp.MustCompile(`(?is)\bfrom\b`).FindStringIndex(selected); from != nil {
			selected = selected[:from[0]]
		}
		for _, withheld := range []string{"token_hash", "delivered_to"} {
			if regexp.MustCompile(`(?i)(?:^|[\s,(])(?:[a-z_][a-z0-9_]*\.)?` + withheld + `\b`).
				MatchString(selected) {
				t.Errorf("the assembled confirm-link section selects %s. It is a live bearer "+
					"credential (or the address it went to) and an Art. 15 package assembled by an "+
					"admin must carry neither:\n%s", withheld, strings.TrimSpace(section.query))
			}
		}
	}
	if probed == 0 {
		t.Fatal("no assembled section reads confirm_token, so this check examined nothing — " +
			"either the section was dropped or it was renamed out of reach")
	}
}
