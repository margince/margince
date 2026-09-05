// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// What a snooze may wait for is spelled in four places, and all four must agree.
//
// values.ReopenConditions() is the Go set every writer builds from, the two
// tables' CHECK constraints are what the database will actually accept, and the
// contract's ReopenCondition enum is what a client may send. A value in the Go
// set that the CHECK refuses is a snooze the product offers and the write
// rejects; one in the CHECK that Go never names is a row no read knows how to
// lift, which sets work aside forever.
//
// Every side is DERIVED — the Go set from the function, the SQL sets from the
// migration text, the wire set from the YAML. Hard-coding the three values here
// would make this file a fifth copy rather than a check on the other four.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The two tables that hold a set-aside. Both were given the column by the same
// migration and both must admit the same values: a rep who can wait for a reply
// on a deal and not on the message that deal is waiting for would be reading
// one product with two different vocabularies.
var reopenConditionTables = []string{"brief_item", "activity_reader_state"}

func TestEveryTableAdmitsExactlyTheConditionsGoCanWrite(t *testing.T) {
	t.Parallel()
	want := make([]string, 0, len(values.ReopenConditions()))
	for _, c := range values.ReopenConditions() {
		want = append(want, string(c))
	}
	slices.Sort(want)

	for _, table := range reopenConditionTables {
		got := reopenCheckValues(t, table)
		if !slices.Equal(got, want) {
			t.Errorf("%s admits %v but values.ReopenConditions() is %v; "+
				"a condition on one side only is either a snooze the write refuses "+
				"or a stored wait no read can lift", table, got, want)
		}
	}
}

func TestTheWireOffersExactlyTheConditionsGoCanWrite(t *testing.T) {
	t.Parallel()
	want := make([]string, 0, len(values.ReopenConditions()))
	for _, c := range values.ReopenConditions() {
		want = append(want, string(c))
	}
	slices.Sort(want)

	got := reopenContractEnum(t)
	if !slices.Equal(got, want) {
		t.Errorf("the contract's ReopenCondition enum is %v but values.ReopenConditions() is %v; "+
			"a value on the wire the server cannot store is a promise no writer keeps", got, want)
	}
}

// reopenCheckValues reads one table's reopen_on CHECK out of the migrations.
//
// It scans EVERY migration rather than the one that introduced the column,
// because a later one may widen the set, and a gate that reads only the first
// would keep passing while the constraint it describes no longer exists.
func reopenCheckValues(t *testing.T, table string) []string {
	t.Helper()
	constraint := table + "_reopen_known"
	// The clause as the migration spells it: reopen_on IN ('a', 'b', 'c').
	clause := regexp.MustCompile(`CONSTRAINT\s+` + regexp.QuoteMeta(constraint) +
		`\s+CHECK\s*\(reopen_on IS NULL OR reopen_on IN \(([^)]*)\)\)`)

	files, err := filepath.Glob(filepath.Join("migrations", "core", "*.up.sql"))
	if err != nil {
		t.Fatalf("listing the migrations: %v", err)
	}
	slices.Sort(files)
	var found []string
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		for _, match := range clause.FindAllStringSubmatch(string(body), -1) {
			// The LAST match wins, so a migration that widens the set is what
			// this gate reports rather than the one that first created it.
			found = quotedValues(match[1])
		}
	}
	if found == nil {
		t.Fatalf("no %s CHECK found in the migrations; the constraint this gate "+
			"protects is gone, and with it every guarantee about what a snooze may wait for", constraint)
	}
	slices.Sort(found)
	return found
}

// quotedValues pulls the single-quoted literals out of an IN list.
func quotedValues(list string) []string {
	quoted := regexp.MustCompile(`'([^']*)'`)
	out := []string{}
	for _, m := range quoted.FindAllStringSubmatch(list, -1) {
		out = append(out, m[1])
	}
	return out
}

// reopenContractEnum reads the shared schema's enum out of the contract.
func reopenContractEnum(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("api", "crm.yaml"))
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Enum []string `yaml:"enum"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	schema, ok := doc.Components.Schemas["ReopenCondition"]
	if !ok {
		t.Fatal("the contract has no ReopenCondition schema; the enum this gate holds is gone")
	}
	out := slices.Clone(schema.Enum)
	// The nullable spellings elsewhere carry a null member; this shared schema
	// does not, and a stray empty would compare unequal for a reason that has
	// nothing to do with drift.
	out = slices.DeleteFunc(out, func(v string) bool { return strings.TrimSpace(v) == "" })
	slices.Sort(out)
	return out
}
