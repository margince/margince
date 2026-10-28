// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// instance/go.mod is the one switch that turns a unit on, while the tree's
// other gates read the units from the folders under extensions/. The two must
// name the same set, or a gate judges a unit that does not run, or skips one
// that does.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// localUnitReplace matches a replace that points a required unit at its folder
// under extensions/, the shape instance/go.mod gives every unit of this tree.
var localUnitReplace = regexp.MustCompile(`=>\s*\.\./extensions/([^\s/]+)\s*$`)

func TestInstanceGoModRequiresExactlyTheUnitsUnderExtensions(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../instance/go.mod")
	if err != nil {
		t.Fatal(err)
	}
	var required []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := localUnitReplace.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			required = append(required, m[1])
		}
	}
	entries, err := os.ReadDir("../extensions")
	if err != nil {
		t.Fatal(err)
	}
	var present []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join("../extensions", e.Name(), "go.mod")); err == nil {
			present = append(present, e.Name())
		}
	}
	if len(present) == 0 {
		t.Fatal("no unit under ../extensions: this gate compares nothing")
	}
	sort.Strings(required)
	sort.Strings(present)
	if strings.Join(required, " ") != strings.Join(present, " ") {
		t.Errorf("instance/go.mod turns on %v, but extensions/ holds %v.\n"+
			"Add a require and a replace onto ../extensions/<name> for each unit, or remove the folder of a unit you turned off.",
			required, present)
	}
}
