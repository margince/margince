// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A `_total` suffix means COUNTER, in both directions.
//
// Prometheus reserves the suffix for monotonically increasing counters, and a
// dashboard author reads the name long before the `# TYPE` line. Four sweep
// gauges carried it — levels read out of river_job at scrape time, numbers that
// go DOWN — which invited rate() and increase() over them. Neither is
// meaningless-looking on a graph: both render as plausible noise, so the
// mistake is silent at exactly the moment somebody is deciding whether tenants
// are being missed.
//
// The reverse matters as much. A counter WITHOUT the suffix is a series nobody
// thinks to rate, read as a level and alerted on as one, which is the same
// error with the roles swapped.
//
// The census is over what this tree EMITS, not over a list of family names: it
// reads the `# TYPE` lines out of the exposition writers themselves, so a
// family added tomorrow is judged the day it is written and one that is deleted
// takes its row with it.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// typeLine matches an emitted `# TYPE <family> <type>` however the writer
// spells it — a Fprintf format string or a WriteString literal — because the
// exposition is assembled both ways in this tree.
var typeLine = regexp.MustCompile(`# TYPE (margince_[a-z0-9_]+) (counter|gauge|histogram|summary|untyped)`)

// familyHeader matches the compose helper that renders a header from a NAME
// PARAMETER. Its type is not in the format string it fills, so the regex above
// cannot see it: writeFamilyHeader emits `gauge` for every caller, which is
// what makes the call site's literal name judgeable here.
var familyHeader = regexp.MustCompile(`writeFamilyHeader\((?:[a-zA-Z0-9_.]+), "(margince_[a-z0-9_]+)"`)

// minFamilies is a floor, not a count. A census that finds nothing passes while
// checking nothing, and these regexes are exactly the kind that stop matching
// when a writer is refactored into a helper. Set well below what the tree
// carries so ordinary deletions do not trip it.
const minFamilies = 20

func TestOnlyACounterCarriesTheTotalSuffix(t *testing.T) {
	t.Parallel()
	families := map[string]string{}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range typeLine.FindAllStringSubmatch(string(body), -1) {
			families[m[1]] = m[2]
		}
		for _, m := range familyHeader.FindAllStringSubmatch(string(body), -1) {
			families[m[1]] = "gauge"
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if len(families) < minFamilies {
		t.Fatalf("only %d metric families found and the tree carries far more — the patterns have gone "+
			"blind, and a census that matches nothing reports PASS", len(families))
	}

	for name, kind := range families {
		total := strings.HasSuffix(name, "_total")
		switch {
		case total && kind != "counter":
			t.Errorf("%s is a %s and carries the _total suffix Prometheus reserves for counters.\n"+
				"Drop the suffix. The # TYPE line is right, and it is not what a dashboard author reads: "+
				"the name is, and this one invites rate() and increase() over a number that goes down — "+
				"which render as plausible noise rather than as an error.", name, kind)
		case !total && kind == "counter":
			t.Errorf("%s is a counter and does not end in _total.\n"+
				"Add the suffix. Without it the series reads as a level, and gets alerted on as one.", name)
		}
	}
}
